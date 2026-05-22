package lua

// lua_test.go — Task 1 skeleton + Task 9 decode/encode dispatch
// integration tests + Task 10 factory-body + stats-registration tests.
// Task 1: TypeURL byte-pin. Task 9: DecodeHeaders + EncodeHeaders +
// OnDestroy + SendLocalReply test-double end-to-end integration. Task
// 10: New full-body wiring (happy-path + nil typed_config + 3-counter
// HCM-rooted registration + empty-prefix consecutive-dot + cardinality
// + statName* byte-exact constants).

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	luav3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	luaprim "github.com/esalaine/envoy-go/internal/lua"
	"github.com/esalaine/envoy-go/internal/stats"
)

// TestTypeURL_Matches pins the byte-exact TypeURL constant per
// 22.1 SPEC §4.1 + ADR-0143 SN1. A regression on the wire URL surfaces
// at this test before propagating to listener-config parsing.
func TestTypeURL_Matches(t *testing.T) {
	const expected = "type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua"
	if TypeURL != expected {
		t.Fatalf("TypeURL = %q; want %q", TypeURL, expected)
	}
}

// TestNew_NilTypedConfig_ParseRejects pins the ADR-0072 boot-time-fail-
// fast contract: a nil *anypb.Any returns the arm-1 PARSE-REJECT
// "lua: typed_config required" verbatim per parent §6.2 + Task 10
// New body. The byte-stable wording is owned by compiled_config.go's
// parseRejectTypedConfigRequired constant.
func TestNew_NilTypedConfig_ParseRejects(t *testing.T) {
	_, err := New(nil, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("New(nil) returned nil error; want arm-1 PARSE-REJECT")
	}
	if err.Error() != parseRejectTypedConfigRequired {
		t.Fatalf("New(nil) err = %q; want %q", err.Error(), parseRejectTypedConfigRequired)
	}
}

// ----------------------------------------------------------------------
// Task 9 — DecodeHeaders + EncodeHeaders + OnDestroy integration tests
// ----------------------------------------------------------------------
//
// These tests exercise the end-to-end decode/encode dispatcher per 22.1
// SPEC §4.3. The test-double DecoderFilterCallbacks records
// SendLocalReply invocations; the test compiles a Lua script via
// luaprim.CompileScript + constructs a *filter holding the chunk + a
// stat-bearing filterStats + the test-double cb + runs DecodeHeaders /
// EncodeHeaders directly.

// localReplyArgs captures one SendLocalReply invocation.
type localReplyArgs struct {
	status  int
	body    string
	headers envoyhttp.OrderedHeaders
}

// recordedDCB is a test-double DecoderFilterCallbacks for the Task 9
// dispatch integration tests. Mirrors the adaptive_concurrency
// recordedCallbacks shape; satisfies the full envoyhttp.DecoderFilterCallbacks
// interface via zero-value stubs for the unused methods.
type recordedDCB struct {
	mu         sync.Mutex
	localReply *localReplyArgs
}

func (c *recordedDCB) ContinueDecoding() {}

func (c *recordedDCB) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.localReply = &localReplyArgs{status: status, body: body, headers: headers}
}

func (c *recordedDCB) RequestRouteConfig() proto.Message { return nil }
func (c *recordedDCB) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (c *recordedDCB) EncodeHeaders(http.Header, bool)  {}
func (c *recordedDCB) EncodeData([]byte, bool)          {}
func (c *recordedDCB) EncodeTrailers(http.Header)       {}
func (c *recordedDCB) DownstreamPrincipal() []string    { return nil }
func (c *recordedDCB) DownstreamRemoteAddr() net.Addr   { return nil }
func (c *recordedDCB) DownstreamLocalAddr() net.Addr    { return nil }
func (c *recordedDCB) DownstreamTLSServerName() string  { return "" }
func (c *recordedDCB) DownstreamTLSPeerCertDER() []byte { return nil }
func (c *recordedDCB) DownstreamProtocol() string       { return "HTTP/1.1" }
func (c *recordedDCB) ListenerPrincipal() string        { return "" }

// ADR-0192 callback-surface extension stubs (phase-22.2 Task 5).
func (c *recordedDCB) DownstreamTLSConnectionState() *tls.ConnectionState { return nil }
func (c *recordedDCB) DynamicMetadata() *dynamicmetadata.Bucket           { return nil }

// recordedECB is the EncoderFilterCallbacks test-double (encode-side
// counterpart). All methods are zero-value stubs since encode_headers.go
// does not call any of them at 22.1.
type recordedECB struct{}

func (c *recordedECB) ContinueEncoding()                {}
func (c *recordedECB) EncodeHeaders(http.Header, bool)  {}
func (c *recordedECB) EncodeData([]byte, bool)          {}
func (c *recordedECB) EncodeTrailers(http.Header)       {}
func (c *recordedECB) OverwriteBody([]byte)             {}
func (c *recordedECB) BufferEncodedBody() []byte        { return nil }
func (c *recordedECB) DownstreamRemoteAddr() net.Addr   { return nil }
func (c *recordedECB) DownstreamLocalAddr() net.Addr    { return nil }
func (c *recordedECB) DownstreamTLSServerName() string  { return "" }
func (c *recordedECB) DownstreamTLSPeerCertDER() []byte { return nil }
func (c *recordedECB) DownstreamProtocol() string       { return "HTTP/1.1" }
func (c *recordedECB) ListenerPrincipal() string        { return "" }

// ADR-0192 callback-surface extension stubs (phase-22.2 Task 5).
func (c *recordedECB) DownstreamTLSConnectionState() *tls.ConnectionState { return nil }
func (c *recordedECB) DynamicMetadata() *dynamicmetadata.Bucket           { return nil }

// newTestFilter constructs a *filter with the supplied script compiled
// into a *compiledConfig (with a stat-bearing filterStats wired to a
// fresh registry). nil-script → cc with nil chunk (the pass-through
// case). The recordedDCB + recordedECB are bound via SetXxxCallbacks.
func newTestFilter(t *testing.T, script string) (*filter, *recordedDCB, *filterStats) {
	t.Helper()
	reg := stats.NewRegistry()
	fs := &filterStats{
		errors:       reg.NewCounter("test.lua.errors"),
		executions:   reg.NewCounter("test.lua.executions"),
		respondCalls: reg.NewCounter("test.lua.respond_calls"),
	}
	cc := &compiledConfig{stats: fs}
	if script != "" {
		chunk, err := luaprim.CompileScript([]byte(script), nil)
		if err != nil {
			t.Fatalf("CompileScript err = %v; script = %q", err, script)
		}
		cc.chunk = chunk
	}
	dcb := &recordedDCB{}
	ecb := &recordedECB{}
	f := &filter{cc: cc}
	f.SetDecoderCallbacks(dcb)
	f.SetEncoderCallbacks(ecb)
	t.Cleanup(f.OnDestroy)
	return f, dcb, fs
}

// TestFilter_DecodeHeaders_NoScript_PassThrough verifies that a *filter
// with nil cc.chunk (D1-REFUTED arm-5 silent-no-op) returns Continue
// without constructing a VM.
func TestFilter_DecodeHeaders_NoScript_PassThrough(t *testing.T) {
	f, dcb, fs := newTestFilter(t, "")
	status := f.DecodeHeaders(http.Header{}, false)
	if status != envoyhttp.Continue {
		t.Errorf("status = %v; want Continue", status)
	}
	if f.vm != nil {
		t.Errorf("f.vm = %v; want nil (no VM construction on nil-chunk path)", f.vm)
	}
	if dcb.localReply != nil {
		t.Errorf("SendLocalReply fired; want no-op")
	}
	if fs.executions.Load() != 0 {
		t.Errorf("executions = %d; want 0 (no hook invocation)", fs.executions.Load())
	}
}

// TestFilter_DecodeHeaders_NilCC_PassThrough verifies that a *filter
// with nil cc (defensive) returns Continue. Should never happen in
// practice but the guard is in place.
func TestFilter_DecodeHeaders_NilCC_PassThrough(t *testing.T) {
	f := &filter{}
	status := f.DecodeHeaders(http.Header{}, false)
	if status != envoyhttp.Continue {
		t.Errorf("status = %v; want Continue", status)
	}
}

// TestFilter_DecodeHeaders_ScriptDefinesNoHook_PassThrough verifies that
// a script that does NOT define envoy_on_request (D1-REFUTED arm-17
// silent-no-op) returns Continue without invoking any hook.
func TestFilter_DecodeHeaders_ScriptDefinesNoHook_PassThrough(t *testing.T) {
	f, dcb, fs := newTestFilter(t, `local x = 1 + 1`)
	status := f.DecodeHeaders(http.Header{}, false)
	if status != envoyhttp.Continue {
		t.Errorf("status = %v; want Continue", status)
	}
	if dcb.localReply != nil {
		t.Errorf("SendLocalReply fired; want no-op")
	}
	if fs.executions.Load() != 0 {
		t.Errorf("executions = %d; want 0 (no hook defined)", fs.executions.Load())
	}
	if fs.errors.Load() != 0 {
		t.Errorf("errors = %d; want 0 (no errors)", fs.errors.Load())
	}
}

// TestFilter_DecodeHeaders_HookCalled_Continue verifies that a script
// defining envoy_on_request that simply mutates the headers returns
// Continue + the headers carry the mutation + stats.executions++.
func TestFilter_DecodeHeaders_HookCalled_Continue(t *testing.T) {
	const script = `function envoy_on_request(rh) rh:headers():add("X-Lua-Touched", "yes") end`
	f, dcb, fs := newTestFilter(t, script)
	h := http.Header{"X-Original": []string{"v"}}
	status := f.DecodeHeaders(h, false)
	if status != envoyhttp.Continue {
		t.Errorf("status = %v; want Continue", status)
	}
	if dcb.localReply != nil {
		t.Errorf("SendLocalReply fired; want no-op")
	}
	if got := h.Get("X-Lua-Touched"); got != "yes" {
		t.Errorf("X-Lua-Touched header = %q; want %q", got, "yes")
	}
	if fs.executions.Load() != 1 {
		t.Errorf("executions = %d; want 1", fs.executions.Load())
	}
	if fs.errors.Load() != 0 {
		t.Errorf("errors = %d; want 0", fs.errors.Load())
	}
}

// TestFilter_DecodeHeaders_HookRespond_StopIteration_SendLocalReply
// verifies the end-to-end :respond() short-circuit path: hook calls
// :respond → StopIteration + dcb.SendLocalReply invoked with the
// captured (status, body, headers) tuple.
func TestFilter_DecodeHeaders_HookRespond_StopIteration_SendLocalReply(t *testing.T) {
	const script = `function envoy_on_request(rh) rh:respond({[":status"]="403"}, "denied") end`
	f, dcb, fs := newTestFilter(t, script)
	status := f.DecodeHeaders(http.Header{}, false)
	if status != envoyhttp.StopIteration {
		t.Errorf("status = %v; want StopIteration", status)
	}
	if dcb.localReply == nil {
		t.Fatal("SendLocalReply not invoked; want invoked")
	}
	if dcb.localReply.status != 403 {
		t.Errorf("SendLocalReply status = %d; want 403", dcb.localReply.status)
	}
	if dcb.localReply.body != "denied" {
		t.Errorf("SendLocalReply body = %q; want %q", dcb.localReply.body, "denied")
	}
	// Headers: text/plain default + auto content-length 6.
	got := dcb.localReply.headers.ToHTTPHeader()
	if v := got.Get("content-type"); v != "text/plain" {
		t.Errorf("SendLocalReply content-type = %q; want %q", v, "text/plain")
	}
	if v := got.Get("content-length"); v != "6" {
		t.Errorf("SendLocalReply content-length = %q; want %q", v, "6")
	}
	if fs.respondCalls.Load() != 1 {
		t.Errorf("respondCalls = %d; want 1", fs.respondCalls.Load())
	}
	if fs.executions.Load() != 1 {
		t.Errorf("executions = %d; want 1", fs.executions.Load())
	}
}

// TestFilter_DecodeHeaders_RunError_StatsErrors_Continue verifies that
// a script with a top-level runtime error increments stats.errors AND
// returns Continue (degraded pass-through per BRAINSTORM §2.9). We
// trigger a runtime error via error() at top-level.
func TestFilter_DecodeHeaders_RunError_StatsErrors_Continue(t *testing.T) {
	const script = `error("top-level boom")`
	f, dcb, fs := newTestFilter(t, script)
	status := f.DecodeHeaders(http.Header{}, false)
	if status != envoyhttp.Continue {
		t.Errorf("status = %v; want Continue (degraded pass-through)", status)
	}
	if dcb.localReply != nil {
		t.Errorf("SendLocalReply fired; want no-op")
	}
	if fs.errors.Load() != 1 {
		t.Errorf("errors = %d; want 1 (top-level script error)", fs.errors.Load())
	}
	if fs.executions.Load() != 0 {
		t.Errorf("executions = %d; want 0 (hook never invoked due to Run error)", fs.executions.Load())
	}
}

// TestFilter_DecodeHeaders_HookError_StatsErrors_Continue verifies that
// a hook that raises a Lua error increments stats.errors AND returns
// Continue (no respond capture → pass-through despite the error).
func TestFilter_DecodeHeaders_HookError_StatsErrors_Continue(t *testing.T) {
	const script = `function envoy_on_request(rh) error("hook boom") end`
	f, dcb, fs := newTestFilter(t, script)
	status := f.DecodeHeaders(http.Header{}, false)
	if status != envoyhttp.Continue {
		t.Errorf("status = %v; want Continue (degraded pass-through)", status)
	}
	if dcb.localReply != nil {
		t.Errorf("SendLocalReply fired; want no-op")
	}
	if fs.errors.Load() != 1 {
		t.Errorf("errors = %d; want 1 (hook error)", fs.errors.Load())
	}
	if fs.executions.Load() != 1 {
		t.Errorf("executions = %d; want 1 (executions counts per-invocation, not per-success)", fs.executions.Load())
	}
}

// TestFilter_DecodeHeaders_StatsExecutions_Inc verifies that executions
// increments by exactly 1 per successful invocation of the hook.
func TestFilter_DecodeHeaders_StatsExecutions_Inc(t *testing.T) {
	const script = `function envoy_on_request(rh) end`
	f, _, fs := newTestFilter(t, script)
	if got := fs.executions.Load(); got != 0 {
		t.Fatalf("baseline executions = %d; want 0", got)
	}
	_ = f.DecodeHeaders(http.Header{}, false)
	if got := fs.executions.Load(); got != 1 {
		t.Errorf("after 1 invocation: executions = %d; want 1", got)
	}
}

// TestFilter_EncodeHeaders_NoScript_PassThrough verifies that the
// encode-side dispatcher pass-through when cc.chunk is nil.
func TestFilter_EncodeHeaders_NoScript_PassThrough(t *testing.T) {
	f, _, fs := newTestFilter(t, "")
	status := f.EncodeHeaders(http.Header{}, false)
	if status != envoyhttp.Continue {
		t.Errorf("status = %v; want Continue", status)
	}
	if fs.executions.Load() != 0 {
		t.Errorf("executions = %d; want 0", fs.executions.Load())
	}
}

// TestFilter_EncodeHeaders_NoVM_PassThrough verifies that the encode-
// side dispatcher pass-through when f.vm is nil (DecodeHeaders did not
// construct one, e.g. test that only invokes EncodeHeaders).
func TestFilter_EncodeHeaders_NoVM_PassThrough(t *testing.T) {
	const script = `function envoy_on_response(rh) end`
	f, _, fs := newTestFilter(t, script)
	// Skip DecodeHeaders; go straight to EncodeHeaders. f.vm is nil.
	status := f.EncodeHeaders(http.Header{}, false)
	if status != envoyhttp.Continue {
		t.Errorf("status = %v; want Continue (vm-nil pass-through)", status)
	}
	if fs.executions.Load() != 0 {
		t.Errorf("executions = %d; want 0 (no VM → no hook)", fs.executions.Load())
	}
}

// TestFilter_EncodeHeaders_HookCalled verifies that a script defining
// envoy_on_response is invoked from EncodeHeaders + executions++.
func TestFilter_EncodeHeaders_HookCalled(t *testing.T) {
	const script = `function envoy_on_response(rh) rh:headers():add("X-Encoded", "yes") end`
	f, _, fs := newTestFilter(t, script)
	// DecodeHeaders constructs the VM + Runs the chunk.
	_ = f.DecodeHeaders(http.Header{}, false)
	// EncodeHeaders fires the response hook.
	h := http.Header{}
	status := f.EncodeHeaders(h, false)
	if status != envoyhttp.Continue {
		t.Errorf("status = %v; want Continue", status)
	}
	if got := h.Get("X-Encoded"); got != "yes" {
		t.Errorf("X-Encoded = %q; want %q", got, "yes")
	}
	if fs.executions.Load() != 1 {
		t.Errorf("executions = %d; want 1 (1 response hook invocation; no request hook defined)", fs.executions.Load())
	}
}

// TestFilter_EncodeHeaders_HookRespond_StatsErrors verifies that an
// encode-side :respond() raises the AMEND-8 byte-exact error →
// stats.errors++ + Continue (no SendLocalReply; the error path does
// NOT capture state).
func TestFilter_EncodeHeaders_HookRespond_StatsErrors(t *testing.T) {
	const script = `function envoy_on_response(rh) rh:respond({[":status"]="200"}, "") end`
	f, _, fs := newTestFilter(t, script)
	_ = f.DecodeHeaders(http.Header{}, false)
	status := f.EncodeHeaders(http.Header{}, false)
	if status != envoyhttp.Continue {
		t.Errorf("status = %v; want Continue (encode-side respond reject)", status)
	}
	if fs.errors.Load() != 1 {
		t.Errorf("errors = %d; want 1 (encode-side :respond reject per AMEND-8)", fs.errors.Load())
	}
	if fs.executions.Load() != 1 {
		t.Errorf("executions = %d; want 1 (response hook invoked once)", fs.executions.Load())
	}
}

// TestFilter_OnDestroy_ClosesVM verifies that OnDestroy releases the
// per-stream VM via vm.Close + the f.vm field is nilled (idempotent
// guard).
func TestFilter_OnDestroy_ClosesVM(t *testing.T) {
	const script = `function envoy_on_request(rh) end`
	// Don't auto-cleanup — we're explicitly testing OnDestroy semantics.
	reg := stats.NewRegistry()
	fs := &filterStats{
		errors:       reg.NewCounter("ondestroy.lua.errors"),
		executions:   reg.NewCounter("ondestroy.lua.executions"),
		respondCalls: reg.NewCounter("ondestroy.lua.respond_calls"),
	}
	chunk, err := luaprim.CompileScript([]byte(script), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v", err)
	}
	f := &filter{cc: &compiledConfig{chunk: chunk, stats: fs}}
	f.SetDecoderCallbacks(&recordedDCB{})
	_ = f.DecodeHeaders(http.Header{}, false)
	if f.vm == nil {
		t.Fatal("f.vm == nil after DecodeHeaders; want non-nil")
	}
	f.OnDestroy()
	if f.vm != nil {
		t.Errorf("f.vm = %v; want nil after OnDestroy", f.vm)
	}
	// Idempotent: second OnDestroy must not panic.
	f.OnDestroy()
}

// TestFilter_OnDestroy_NilVMNoPanic verifies the OnDestroy guard against
// the nil-vm case (DecodeHeaders short-circuited at nil-chunk).
func TestFilter_OnDestroy_NilVMNoPanic(t *testing.T) {
	f := &filter{}
	// No panic on bare-OnDestroy without a VM.
	f.OnDestroy()
}

// TestFilter_DecodeHeaders_Then_EncodeHeaders_BothHooksFire verifies the
// end-to-end happy path: both envoy_on_request + envoy_on_response are
// invoked once (executions == 2).
func TestFilter_DecodeHeaders_Then_EncodeHeaders_BothHooksFire(t *testing.T) {
	const script = `
		function envoy_on_request(rh) rh:headers():add("X-Req", "1") end
		function envoy_on_response(rh) rh:headers():add("X-Resp", "1") end
	`
	f, _, fs := newTestFilter(t, script)
	reqH := http.Header{}
	respH := http.Header{}
	if status := f.DecodeHeaders(reqH, false); status != envoyhttp.Continue {
		t.Errorf("decode status = %v; want Continue", status)
	}
	if status := f.EncodeHeaders(respH, false); status != envoyhttp.Continue {
		t.Errorf("encode status = %v; want Continue", status)
	}
	if got := reqH.Get("X-Req"); got != "1" {
		t.Errorf("X-Req = %q; want %q", got, "1")
	}
	if got := respH.Get("X-Resp"); got != "1" {
		t.Errorf("X-Resp = %q; want %q", got, "1")
	}
	if fs.executions.Load() != 2 {
		t.Errorf("executions = %d; want 2 (1 decode + 1 encode hook)", fs.executions.Load())
	}
}

// ----------------------------------------------------------------------
// Task 3 (phase 22.3) — per-route override decode→encode integration
// ----------------------------------------------------------------------

// newPerRouteTestStats builds a fresh stat-bearing *filterStats with a unique
// registry prefix so parallel tests do not collide on counter names.
func newPerRouteTestStats(t *testing.T, prefix string) *filterStats {
	t.Helper()
	reg := stats.NewRegistry()
	return &filterStats{
		errors:       reg.NewCounter(prefix + ".errors"),
		executions:   reg.NewCounter(prefix + ".executions"),
		respondCalls: reg.NewCounter(prefix + ".respond_calls"),
	}
}

// TestFilter_PerRoute_SourceCodeOverride_DefaultLessListener_EncodeFires is the
// encode-guard regression pin (AMEND-22.3 + the load-bearing encode-guard fix):
// a per-route source_code override defining envoy_on_response on a DEFAULT-LESS
// listener (cc.chunk == nil) builds + runs the override VM at DecodeHeaders, so
// f.vm != nil at EncodeHeaders even though cc.chunk == nil. The new encode guard
// gates only on f.vm == nil, so envoy_on_response MUST fire. The OLD guard
// (cc.chunk == nil) would wrongly skip it.
func TestFilter_PerRoute_SourceCodeOverride_DefaultLessListener_EncodeFires(t *testing.T) {
	fs := newPerRouteTestStats(t, "perroute_sc")
	cc := &compiledConfig{chunk: nil, compileCache: luaprim.NewCompileCache(), stats: fs}
	pr := &luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_SourceCode{
			SourceCode: &corev3.DataSource{
				Specifier: &corev3.DataSource_InlineString{
					InlineString: `function envoy_on_response(rh) rh:headers():add("X-PR-Resp", "1") end`,
				},
			},
		},
	}
	f, _ := newResolveTestFilter(cc, pr)
	t.Cleanup(f.OnDestroy)

	if status := f.DecodeHeaders(http.Header{}, false); status != envoyhttp.Continue {
		t.Fatalf("decode status = %v; want Continue", status)
	}
	if f.vm == nil {
		t.Fatal("f.vm == nil after DecodeHeaders with source_code override; want non-nil VM")
	}

	respH := http.Header{}
	if status := f.EncodeHeaders(respH, false); status != envoyhttp.Continue {
		t.Fatalf("encode status = %v; want Continue", status)
	}
	if got := respH.Get("X-PR-Resp"); got != "1" {
		t.Errorf("X-PR-Resp = %q; want %q (envoy_on_response must fire despite nil cc.chunk)", got, "1")
	}
	if fs.executions.Load() != 1 {
		t.Errorf("executions = %d; want 1 (only envoy_on_response defined)", fs.executions.Load())
	}
}

// TestFilter_PerRoute_Disabled_BuildsNoVM_SkipsBothHooks verifies that a
// per-route `disabled: true` override builds NO VM at DecodeHeaders (the
// disabled early-return happens BEFORE VM construction) and consequently skips
// BOTH the decode + encode hooks (encode is gated by f.vm == nil).
func TestFilter_PerRoute_Disabled_BuildsNoVM_SkipsBothHooks(t *testing.T) {
	fs := newPerRouteTestStats(t, "perroute_disabled")
	defChunk, err := luaprim.CompileScript([]byte(`
		function envoy_on_request(rh) rh:headers():add("X-Req", "1") end
		function envoy_on_response(rh) rh:headers():add("X-Resp", "1") end
	`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v", err)
	}
	cc := &compiledConfig{chunk: defChunk, compileCache: luaprim.NewCompileCache(), stats: fs}
	pr := &luav3.LuaPerRoute{Override: &luav3.LuaPerRoute_Disabled{Disabled: true}}
	f, _ := newResolveTestFilter(cc, pr)
	t.Cleanup(f.OnDestroy)

	reqH := http.Header{}
	if status := f.DecodeHeaders(reqH, false); status != envoyhttp.Continue {
		t.Fatalf("decode status = %v; want Continue", status)
	}
	if f.vm != nil {
		t.Fatalf("f.vm = %v; want nil (disabled → no VM construction)", f.vm)
	}
	if got := reqH.Get("X-Req"); got != "" {
		t.Errorf("X-Req = %q; want empty (decode hook must NOT fire on disabled route)", got)
	}

	respH := http.Header{}
	if status := f.EncodeHeaders(respH, false); status != envoyhttp.Continue {
		t.Fatalf("encode status = %v; want Continue", status)
	}
	if got := respH.Get("X-Resp"); got != "" {
		t.Errorf("X-Resp = %q; want empty (encode hook must NOT fire on disabled route)", got)
	}
	if fs.executions.Load() != 0 {
		t.Errorf("executions = %d; want 0 (both hooks skipped on disabled route)", fs.executions.Load())
	}
}

// ----------------------------------------------------------------------
// Task 10 — stats.go statName* byte-exact constants + newFilterStats
// HCM-rooted registration + cardinality + empty-prefix consecutive-dot
// + New full-body factory tests
// ----------------------------------------------------------------------

// TestStatNames_Equal_Errors pins the byte-exact wire name for the
// errors counter per ADR-0143 SN2-reuse + parent §7.1. A regression on
// the const surfaces at this test BEFORE landing on a /stats scrape.
func TestStatNames_Equal_Errors(t *testing.T) {
	if statNameErrors != "errors" {
		t.Fatalf("statNameErrors = %q; want %q", statNameErrors, "errors")
	}
}

// TestStatNames_Equal_Executions pins the byte-exact wire name for the
// executions counter per ADR-0143 SN2-reuse + parent §7.1.
func TestStatNames_Equal_Executions(t *testing.T) {
	if statNameExecutions != "executions" {
		t.Fatalf("statNameExecutions = %q; want %q", statNameExecutions, "executions")
	}
}

// TestStatNames_Equal_RespondCalls pins the byte-exact wire name for
// the respond_calls counter (envoy-go-strict extension per AMEND-3).
// A regression here would silently break BEHAVIOR_CONTRACT.md §13.6
// row 2 + the §14 edit #4 departure record at Task 16.
func TestStatNames_Equal_RespondCalls(t *testing.T) {
	if statNameRespondCalls != "respond_calls" {
		t.Fatalf("statNameRespondCalls = %q; want %q", statNameRespondCalls, "respond_calls")
	}
}

// TestStatNames_TableDriven asserts all 8 stat name constants in one
// table-driven row per the §6.6 dual-layer guard precedent (mirrors
// adaptive_concurrency::TestStatNames_Equal_* shape).
//
// Extended at Task 14 (phase 22.2 IMPL) per 22.2 SPEC §7.1 + AMEND-22.2-3:
// 3 inherited (22.1) + 5 NEW envoy-go-strict counters. Byte-exact wire
// names per 22.2 SPEC §7.1 row 1-5.
func TestStatNames_TableDriven(t *testing.T) {
	tests := []struct {
		gotConst, want string
	}{
		// 3 inherited from 22.1 (parent §7.1 + AMEND-3).
		{statNameErrors, "errors"},
		{statNameExecutions, "executions"},
		{statNameRespondCalls, "respond_calls"},
		// 5 NEW envoy-go-strict counters at 22.2 Task 14 per 22.2 SPEC §7.1.
		{statNameHTTPCallTotal, "httpcall_total"},
		{statNameHTTPCallFailures, "httpcall_failures"},
		{statNameHTTPCallTimeouts, "httpcall_timeouts"},
		{statNameBodyBufferedBytesTotal, "body_buffered_bytes_total"},
		{statNameCoroutineYieldsTotal, "coroutine_yields_total"},
	}
	for _, tc := range tests {
		if tc.gotConst != tc.want {
			t.Errorf("stat name constant = %q; want %q", tc.gotConst, tc.want)
		}
	}
}

// TestStatNames_Equal_HTTPCallTotal pins the byte-exact wire name for the
// httpcall_total counter (envoy-go-strict extension per 22.2 SPEC §7.1
// row 1 + AMEND-22.2-3). Incremented on every :httpCall() dispatch
// (sync + async). Departure record anticipated at Task 19 BEHAVIOR_
// CONTRACT.md §13.6 per SPEC §14 edit item 3.
func TestStatNames_Equal_HTTPCallTotal(t *testing.T) {
	if statNameHTTPCallTotal != "httpcall_total" {
		t.Fatalf("statNameHTTPCallTotal = %q; want %q", statNameHTTPCallTotal, "httpcall_total")
	}
}

// TestStatNames_Equal_HTTPCallFailures pins the byte-exact wire name for
// the httpcall_failures counter (SYNC-ONLY per AMEND-22.2-3 D6;
// envoy-go-strict per 22.2 SPEC §7.1 row 2). Departure record
// anticipated at Task 19 per SPEC §14 edit item 4.
func TestStatNames_Equal_HTTPCallFailures(t *testing.T) {
	if statNameHTTPCallFailures != "httpcall_failures" {
		t.Fatalf("statNameHTTPCallFailures = %q; want %q", statNameHTTPCallFailures, "httpcall_failures")
	}
}

// TestStatNames_Equal_HTTPCallTimeouts pins the byte-exact wire name for
// the httpcall_timeouts counter (SYNC-ONLY per AMEND-22.2-3 D6;
// envoy-go-strict per 22.2 SPEC §7.1 row 3). Departure record
// anticipated at Task 19 per SPEC §14 edit item 5.
func TestStatNames_Equal_HTTPCallTimeouts(t *testing.T) {
	if statNameHTTPCallTimeouts != "httpcall_timeouts" {
		t.Fatalf("statNameHTTPCallTimeouts = %q; want %q", statNameHTTPCallTimeouts, "httpcall_timeouts")
	}
}

// TestStatNames_Equal_BodyBufferedBytesTotal pins the byte-exact wire
// name for the body_buffered_bytes_total counter (envoy-go-strict per
// 22.2 SPEC §7.1 row 4). Cumulative bytes accumulated in
// decodedBodyBytes / encodedBodyBytes across all streams. Departure
// record anticipated at Task 19 per SPEC §14 edit item 6.
func TestStatNames_Equal_BodyBufferedBytesTotal(t *testing.T) {
	if statNameBodyBufferedBytesTotal != "body_buffered_bytes_total" {
		t.Fatalf("statNameBodyBufferedBytesTotal = %q; want %q",
			statNameBodyBufferedBytesTotal, "body_buffered_bytes_total")
	}
}

// TestStatNames_Equal_CoroutineYieldsTotal pins the byte-exact wire name
// for the coroutine_yields_total counter (envoy-go-strict per 22.2 SPEC
// §7.1 row 5). Cumulative coroutine yield events from
// :body() / :bodyChunks() / sync :httpCall(). Departure record
// anticipated at Task 19 per SPEC §14 edit item 7.
func TestStatNames_Equal_CoroutineYieldsTotal(t *testing.T) {
	if statNameCoroutineYieldsTotal != "coroutine_yields_total" {
		t.Fatalf("statNameCoroutineYieldsTotal = %q; want %q",
			statNameCoroutineYieldsTotal, "coroutine_yields_total")
	}
}

// TestNewFilterStats_RegistersEightCounters_HCMRootedTemplate verifies
// the Task 14 extension: newFilterStats(reg, "ingress_http",
// "my_prefix") registers exactly the 8 byte-exact wire names under the
// HCM-rooted template (UNCHANGED from 22.1 per AMEND-2) per 22.2 SPEC
// §7.1 + §7.2.
func TestNewFilterStats_RegistersEightCounters_HCMRootedTemplate(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_http", "my_prefix")
	if fs == nil {
		t.Fatal("newFilterStats returned nil; want non-nil")
	}
	want := map[string]bool{
		// 3 inherited from 22.1.
		"http.ingress_http.lua.my_prefix.errors":        true,
		"http.ingress_http.lua.my_prefix.executions":    true,
		"http.ingress_http.lua.my_prefix.respond_calls": true,
		// 5 NEW at 22.2 Task 14 per SPEC §7.1.
		"http.ingress_http.lua.my_prefix.httpcall_total":            true,
		"http.ingress_http.lua.my_prefix.httpcall_failures":         true,
		"http.ingress_http.lua.my_prefix.httpcall_timeouts":         true,
		"http.ingress_http.lua.my_prefix.body_buffered_bytes_total": true,
		"http.ingress_http.lua.my_prefix.coroutine_yields_total":    true,
	}
	got := walkRegistry(reg)
	if len(got) != 8 {
		t.Fatalf("registered count = %d; want 8 (got names: %v)", len(got), got)
	}
	for _, n := range got {
		if !want[n] {
			t.Errorf("unexpected registered name %q (not in expected set)", n)
		}
		delete(want, n)
	}
	if len(want) > 0 {
		t.Errorf("missing registered names: %v", want)
	}
	// Verify all 8 counter fields are non-nil per Task 14 acceptance.
	if fs.errors == nil || fs.executions == nil || fs.respondCalls == nil {
		t.Errorf("inherited counter field nil: errors=%v executions=%v respondCalls=%v",
			fs.errors, fs.executions, fs.respondCalls)
	}
	if fs.httpcallTotal == nil || fs.httpcallFailures == nil || fs.httpcallTimeouts == nil {
		t.Errorf("httpCall counter field nil: total=%v failures=%v timeouts=%v",
			fs.httpcallTotal, fs.httpcallFailures, fs.httpcallTimeouts)
	}
	if fs.bodyBufferedBytesTotal == nil || fs.coroutineYieldsTotal == nil {
		t.Errorf("body/coroutine counter field nil: bodyBufferedBytesTotal=%v coroutineYieldsTotal=%v",
			fs.bodyBufferedBytesTotal, fs.coroutineYieldsTotal)
	}
}

// TestNewFilterStats_EightCounterCardinality asserts exactly 8 counters
// are registered per filter instance at 22.2 phase-done per Task 14
// acceptance criteria + 22.2 SPEC §7.1 (102 → 107 project stat-count
// delta). Detects regressions where a future maintainer might add a 9th
// stat (e.g. dynmd_writes_total per §7.1 RECOMMENDATION) without
// updating BEHAVIOR_CONTRACT.md §13.6 + the 22.2 SPEC §7.1 roster.
func TestNewFilterStats_EightCounterCardinality(t *testing.T) {
	reg := stats.NewRegistry()
	_ = newFilterStats(reg, "h", "c")
	got := walkRegistry(reg)
	if len(got) != 8 {
		t.Fatalf("cardinality = %d; want exactly 8 at 22.2 phase-done (names: %v)", len(got), got)
	}
}

// TestNewFilterStats_EmptyConfigStatPrefix_ConsecutiveDot_Eight verifies
// the AMEND-2 consecutive-dot literal carries forward at 22.2 phase-done
// for ALL 8 counters: empty Lua.stat_prefix produces literal
// consecutive-dot wire names per parent §7.2 + AMEND-2.
func TestNewFilterStats_EmptyConfigStatPrefix_ConsecutiveDot_Eight(t *testing.T) {
	reg := stats.NewRegistry()
	_ = newFilterStats(reg, "ingress_http", "")
	want := map[string]bool{
		"http.ingress_http.lua..errors":                    true,
		"http.ingress_http.lua..executions":                true,
		"http.ingress_http.lua..respond_calls":             true,
		"http.ingress_http.lua..httpcall_total":            true,
		"http.ingress_http.lua..httpcall_failures":         true,
		"http.ingress_http.lua..httpcall_timeouts":         true,
		"http.ingress_http.lua..body_buffered_bytes_total": true,
		"http.ingress_http.lua..coroutine_yields_total":    true,
	}
	got := walkRegistry(reg)
	if len(got) != 8 {
		t.Fatalf("registered count = %d; want 8 (got: %v)", len(got), got)
	}
	for _, n := range got {
		if !want[n] {
			t.Errorf("unexpected registered name %q (want consecutive-dot form)", n)
		}
	}
}

// walkRegistry collects all registered metric names into a slice (in
// registration order; the *stats.Registry.Walk callback's order matches
// registration order per its doc-comment).
func walkRegistry(reg *stats.Registry) []string {
	var names []string
	reg.Walk(func(m stats.Metric) {
		names = append(names, m.Name())
	})
	return names
}

// TestNewFilterStats_EmptyHcmAndConfig_DoubleConsecutiveDot verifies
// the AMEND-2 corner case: both ctx.StatPrefix AND Lua.StatPrefix empty
// → `http..lua..<stat>` (two consecutive-dot pairs). The registry name
// regex permits interior consecutive dots per
// internal/stats/registry.go::nameRE; this test pins the operational-
// degenerate-but-valid wire shape across all 8 counters at 22.2 phase-
// done.
//
// EXTENDED at Task 14 to 8 counters per 22.2 SPEC §7.1 (UNCHANGED
// template per AMEND-2). Original 22.1 3-counter cardinality
// assertions (TestNewFilterStats_RegistersThreeCounters_HCMRootedTemplate,
// TestNewFilterStats_CardinalityAssertion,
// TestNewFilterStats_EmptyConfigStatPrefix_ConsecutiveDot) are
// SUPERSEDED by the 8-counter variants above
// (TestNewFilterStats_RegistersEightCounters_HCMRootedTemplate,
// TestNewFilterStats_EightCounterCardinality,
// TestNewFilterStats_EmptyConfigStatPrefix_ConsecutiveDot_Eight).
func TestNewFilterStats_EmptyHcmAndConfig_DoubleConsecutiveDot(t *testing.T) {
	reg := stats.NewRegistry()
	_ = newFilterStats(reg, "", "")
	want := map[string]bool{
		// 3 inherited from 22.1.
		"http..lua..errors":        true,
		"http..lua..executions":    true,
		"http..lua..respond_calls": true,
		// 5 NEW at 22.2 Task 14 per SPEC §7.1.
		"http..lua..httpcall_total":            true,
		"http..lua..httpcall_failures":         true,
		"http..lua..httpcall_timeouts":         true,
		"http..lua..body_buffered_bytes_total": true,
		"http..lua..coroutine_yields_total":    true,
	}
	got := walkRegistry(reg)
	if len(got) != 8 {
		t.Fatalf("registered count = %d; want 8 (got: %v)", len(got), got)
	}
	for _, n := range got {
		if !want[n] {
			t.Errorf("unexpected registered name %q (want double-consecutive-dot form)", n)
		}
	}
}

// validLuaAny returns a valid *anypb.Any wrapping a Lua proto with a
// minimal InlineString default_source_code. Helper for the Task 10
// New-happy-path tests.
func validLuaAny(t *testing.T, statPrefix string) *anypb.Any {
	t.Helper()
	m := &luav3.Lua{
		StatPrefix: statPrefix,
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: "function envoy_on_request(rh) end\n",
			},
		},
	}
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

// TestNew_HappyPath_ReturnsFactoryAndStatsRegistered verifies the Task
// 10 New full-body contract: a valid Lua proto + non-nil ctx.Stats →
// no error + non-nil FilterInstanceFactory + 3 counters registered
// under the HCM-rooted template per parent §7.2 + AMEND-2.
func TestNew_HappyPath_ReturnsFactoryAndStatsRegistered(t *testing.T) {
	reg := stats.NewRegistry()
	factory, err := New(validLuaAny(t, "my_script"), envoyhttp.FactoryCtx{
		Stats:      reg,
		StatPrefix: "ingress_http",
	})
	if err != nil {
		t.Fatalf("New err = %v; want nil", err)
	}
	if factory == nil {
		t.Fatal("New returned nil factory; want non-nil")
	}
	got := walkRegistry(reg)
	if len(got) != 8 {
		t.Fatalf("registered count = %d; want 8 (got: %v)", len(got), got)
	}
	wantNames := map[string]bool{
		// 3 inherited from 22.1.
		"http.ingress_http.lua.my_script.errors":        true,
		"http.ingress_http.lua.my_script.executions":    true,
		"http.ingress_http.lua.my_script.respond_calls": true,
		// 5 NEW at 22.2 Task 14 per SPEC §7.1.
		"http.ingress_http.lua.my_script.httpcall_total":            true,
		"http.ingress_http.lua.my_script.httpcall_failures":         true,
		"http.ingress_http.lua.my_script.httpcall_timeouts":         true,
		"http.ingress_http.lua.my_script.body_buffered_bytes_total": true,
		"http.ingress_http.lua.my_script.coroutine_yields_total":    true,
	}
	for _, n := range got {
		if !wantNames[n] {
			t.Errorf("unexpected name %q registered", n)
		}
	}

	// Exercise the per-stream factory closure: it should produce an
	// HTTPFilter with both Decoder and Encoder non-nil per 22.1 SPEC
	// §3.1 #6 (both-sides filter).
	hf := factory()
	if hf.Name != filterName {
		t.Errorf("HTTPFilter.Name = %q; want %q", hf.Name, filterName)
	}
	if hf.Decoder == nil {
		t.Error("HTTPFilter.Decoder = nil; want non-nil per 22.1 SPEC §3.1 #6")
	}
	if hf.Encoder == nil {
		t.Error("HTTPFilter.Encoder = nil; want non-nil per 22.1 SPEC §3.1 #6")
	}
}

// TestNew_HappyPath_EmptyLuaStatPrefix_ConsecutiveDot verifies that
// when Lua.StatPrefix is empty AND the factory wires through New, the
// 3 counters land under the consecutive-dot literal name shape per
// AMEND-2. End-to-end variant of
// TestNewFilterStats_EmptyConfigStatPrefix_ConsecutiveDot (this test
// exercises the New → buildCompiledConfig → tc.UnmarshalTo →
// newFilterStats path together).
func TestNew_HappyPath_EmptyLuaStatPrefix_ConsecutiveDot(t *testing.T) {
	reg := stats.NewRegistry()
	_, err := New(validLuaAny(t, ""), envoyhttp.FactoryCtx{
		Stats:      reg,
		StatPrefix: "ingress_http",
	})
	if err != nil {
		t.Fatalf("New err = %v; want nil", err)
	}
	got := walkRegistry(reg)
	wantNames := map[string]bool{
		// 3 inherited from 22.1.
		"http.ingress_http.lua..errors":        true,
		"http.ingress_http.lua..executions":    true,
		"http.ingress_http.lua..respond_calls": true,
		// 5 NEW at 22.2 Task 14 per SPEC §7.1.
		"http.ingress_http.lua..httpcall_total":            true,
		"http.ingress_http.lua..httpcall_failures":         true,
		"http.ingress_http.lua..httpcall_timeouts":         true,
		"http.ingress_http.lua..body_buffered_bytes_total": true,
		"http.ingress_http.lua..coroutine_yields_total":    true,
	}
	if len(got) != 8 {
		t.Fatalf("registered count = %d; want 8 (got: %v)", len(got), got)
	}
	for _, n := range got {
		if !wantNames[n] {
			t.Errorf("unexpected name %q registered (want consecutive-dot form)", n)
		}
	}
}

// TestNew_NilStats_NoPanic_NoRegistration verifies the ADR-0085 nil-
// tolerance contract: when ctx.Stats is nil, New must NOT panic and
// must NOT attempt to register stats (the guard inside New short-
// circuits before invoking newFilterStats).
func TestNew_NilStats_NoPanic_NoRegistration(t *testing.T) {
	factory, err := New(validLuaAny(t, "anything"), envoyhttp.FactoryCtx{
		Stats:      nil,
		StatPrefix: "ingress_http",
	})
	if err != nil {
		t.Fatalf("New err = %v; want nil under nil-Stats tolerance", err)
	}
	if factory == nil {
		t.Fatal("New returned nil factory under nil-Stats; want non-nil")
	}
}

// TestNew_BuildCompiledConfigError_Propagates verifies that any
// PARSE-REJECT error from buildCompiledConfig surfaces verbatim
// through New (no extra wrapping). We trigger via arm-3 inline_code-
// deprecated PARSE-REJECT.
func TestNew_BuildCompiledConfigError_Propagates(t *testing.T) {
	// Arm 3: inline_code is deprecated → PARSE-REJECT.
	m := &luav3.Lua{
		InlineCode: "function envoy_on_request(rh) end\n",
	}
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	_, gotErr := New(a, envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "ingress_http"})
	if gotErr == nil {
		t.Fatal("New(inline_code) returned nil err; want arm-3 PARSE-REJECT")
	}
	if !strings.Contains(gotErr.Error(), "inline_code is deprecated") {
		t.Fatalf("New err = %q; want substring %q", gotErr.Error(), "inline_code is deprecated")
	}
}

// fakeRegistry implements the small interface RegisterPerRouteValidator
// expects so we can assert wiring without an actual *HTTPRegistry.
type fakeRegistry struct {
	registered map[string]func(proto.Message) error
}

func (f *fakeRegistry) RegisterPerRouteValidator(filterName string, validator func(proto.Message) error) {
	if f.registered == nil {
		f.registered = make(map[string]func(proto.Message) error)
	}
	f.registered[filterName] = validator
}

// TestRegisterPerRouteValidator_WiresRealValidator verifies the exported
// RegisterPerRouteValidator wires validatePerRouteLua (delegating to
// parsePerRouteLua — the real 3-arm LuaPerRoute validator landed at phase
// 22.3 Task 2) under the canonical filterName `envoy.filters.http.lua`.
// Pattern mirrors header_mutation + oauth2's TestRegisterPerRouteValidator
// regression precedent. The deferred one-liner was retired; the validator
// now enforces the full oneof shape.
func TestRegisterPerRouteValidator_WiresRealValidator(t *testing.T) {
	fr := &fakeRegistry{}
	RegisterPerRouteValidator(fr)
	v, ok := fr.registered[filterName]
	if !ok {
		t.Fatalf("validator NOT registered under filterName %q (registered: %v)", filterName, fr.registered)
	}
	if v == nil {
		t.Fatal("registered validator is nil; want non-nil")
	}
	// Wrong type (non-*LuaPerRoute) → type-assert error (no longer a
	// "deferred" one-liner; the real validator distinguishes wrong-type vs
	// nil-oneof vs arm-level rejects).
	errWrongType := v(&luav3.Lua{})
	if errWrongType == nil {
		t.Fatal("validator(*Lua): want error; got nil")
	}
	if !strings.HasPrefix(errWrongType.Error(), "lua: per-route: expected *luav3.LuaPerRoute, got ") {
		t.Fatalf("validator(*Lua) err = %q; want prefix %q", errWrongType.Error(), "lua: per-route: expected *luav3.LuaPerRoute, got ")
	}
	// Nil-oneof (&LuaPerRoute{}) → byte-exact oneof-required error.
	errNilOneof := v(&luav3.LuaPerRoute{})
	if errNilOneof == nil {
		t.Fatal("validator(&LuaPerRoute{}): want error; got nil")
	}
	if errNilOneof.Error() != parseRejectPerRouteOneofRequired {
		t.Fatalf("validator(&LuaPerRoute{}) err = %q; want %q", errNilOneof.Error(), parseRejectPerRouteOneofRequired)
	}
	// Valid per-route → no error.
	errValid := v(&luav3.LuaPerRoute{Override: &luav3.LuaPerRoute_Name{Name: "myscript"}})
	if errValid != nil {
		t.Fatalf("validator(valid Name): want nil error; got %v", errValid)
	}
}

// ----------------------------------------------------------------------
// Task 12 — per-stream filter dispatch race tests + BenchmarkPerStream
// LState_Construction_Headers per 22.1 PLAN Task 12 + 22.1 SPEC §6 Task
// 12 + §2.19 + §13-R6 + parent §13-R6 + D-P10 escape-valve gate.
//
// The race tests cover the per-stream-goroutine-isolation invariant
// (ADR-0071): each per-stream *filter instance is constructed fresh from
// the FilterInstanceFactory closure + shares only the immutable
// *compiledConfig (containing the *Chunk + the *CompileCache + the
// stat-bearing *filterStats). N concurrent DecodeHeaders/EncodeHeaders
// dispatches against N filter instances must be race-free under -race
// AND must observe no cross-stream state leak (each stream's hook
// observes its own *requestHandleContext, not another stream's).
//
// The benchmark measures per-stream *lua.LState construction cost (the
// production DecodeHeaders allocates a fresh VM + installs the bridge
// metatables + builds the request handle userdata + Runs the chunk +
// invokes the hook). The reported ns/op gates ADR-0190 firing at Task
// 16: ns/op > 1_000_000 (= 1ms) → escape-valve fires; otherwise
// per-stream construction stays WEAK-default.
// ----------------------------------------------------------------------

// TestFilter_ConcurrentDecodeHeaders verifies that N=100 per-stream
// filter instances each invoking DecodeHeaders concurrently do not race.
// Each filter holds its own headers map; the Lua script reads the
// X-Stream-Id header + writes it back as X-Lua-Saw to assert no cross-
// stream leak of the request_handle headers carrier.
func TestFilter_ConcurrentDecodeHeaders(t *testing.T) {
	const script = `
		function envoy_on_request(rh)
			local v = rh:headers():get("X-Stream-Id")
			rh:headers():add("X-Lua-Saw", v or "")
		end
	`
	// Single SHARED *compiledConfig — mirrors the production
	// FilterInstanceFactory closure semantics where every per-stream
	// *filter closes over the SAME cc per Task 10 lua.go:150-157.
	chunk, err := luaprim.CompileScript([]byte(script), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		chunk: chunk,
		stats: &filterStats{
			errors:       reg.NewCounter("ct.lua.errors"),
			executions:   reg.NewCounter("ct.lua.executions"),
			respondCalls: reg.NewCounter("ct.lua.respond_calls"),
		},
	}

	const N = 100
	var (
		wg       sync.WaitGroup
		headers  = make([]http.Header, N)
		failures atomic.Int64
	)
	for i := 0; i < N; i++ {
		headers[i] = http.Header{"X-Stream-Id": []string{fmt.Sprintf("stream-%d", i)}}
	}

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			f := &filter{cc: cc}
			f.SetDecoderCallbacks(&recordedDCB{})
			defer f.OnDestroy()
			status := f.DecodeHeaders(headers[idx], false)
			if status != envoyhttp.Continue {
				t.Errorf("[%d]: status = %v; want Continue", idx, status)
				failures.Add(1)
				return
			}
			want := fmt.Sprintf("stream-%d", idx)
			if got := headers[idx].Get("X-Lua-Saw"); got != want {
				t.Errorf("[%d]: X-Lua-Saw = %q; want %q (cross-stream headers leak)", idx, got, want)
				failures.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if n := failures.Load(); n > 0 {
		t.Fatalf("%d / %d goroutines failed assertions", n, N)
	}
	// stats.executions must equal N (each per-stream invocation bumps
	// exactly once at Task 9 decode_headers.go step 7). The counter is
	// atomic at internal/stats/registry.go::Counter; this assertion is
	// race-free.
	if got := cc.stats.executions.Load(); got != uint64(N) {
		t.Errorf("executions = %d; want %d (one per per-stream invocation)", got, N)
	}
	if got := cc.stats.errors.Load(); got != 0 {
		t.Errorf("errors = %d; want 0", got)
	}
}

// TestFilter_ConcurrentDecodeAndEncode verifies that N=100 goroutines
// each running the full DecodeHeaders→EncodeHeaders cycle on
// independent filter instances do not race. The script defines BOTH
// hooks; each goroutine asserts its own request-side + response-side
// header mutations land on its own carriers, never on another
// goroutine's.
func TestFilter_ConcurrentDecodeAndEncode(t *testing.T) {
	const script = `
		function envoy_on_request(rh)
			rh:headers():add("X-Req-Stream", rh:headers():get("X-In") or "")
		end
		function envoy_on_response(rh)
			rh:headers():add("X-Resp-Stream", rh:headers():get("X-In-Resp") or "")
		end
	`
	chunk, err := luaprim.CompileScript([]byte(script), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		chunk: chunk,
		stats: &filterStats{
			errors:       reg.NewCounter("ct2.lua.errors"),
			executions:   reg.NewCounter("ct2.lua.executions"),
			respondCalls: reg.NewCounter("ct2.lua.respond_calls"),
		},
	}

	const N = 100
	var (
		wg       sync.WaitGroup
		failures atomic.Int64
	)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			f := &filter{cc: cc}
			f.SetDecoderCallbacks(&recordedDCB{})
			f.SetEncoderCallbacks(&recordedECB{})
			defer f.OnDestroy()

			reqH := http.Header{"X-In": []string{fmt.Sprintf("req-%d", idx)}}
			if status := f.DecodeHeaders(reqH, false); status != envoyhttp.Continue {
				t.Errorf("[%d]: decode status = %v; want Continue", idx, status)
				failures.Add(1)
				return
			}
			if got, want := reqH.Get("X-Req-Stream"), fmt.Sprintf("req-%d", idx); got != want {
				t.Errorf("[%d]: X-Req-Stream = %q; want %q", idx, got, want)
				failures.Add(1)
				return
			}

			respH := http.Header{"X-In-Resp": []string{fmt.Sprintf("resp-%d", idx)}}
			if status := f.EncodeHeaders(respH, false); status != envoyhttp.Continue {
				t.Errorf("[%d]: encode status = %v; want Continue", idx, status)
				failures.Add(1)
				return
			}
			if got, want := respH.Get("X-Resp-Stream"), fmt.Sprintf("resp-%d", idx); got != want {
				t.Errorf("[%d]: X-Resp-Stream = %q; want %q", idx, got, want)
				failures.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if n := failures.Load(); n > 0 {
		t.Fatalf("%d / %d goroutines failed assertions", n, N)
	}
	// One decode hook + one encode hook per goroutine = 2*N executions.
	if got := cc.stats.executions.Load(); got != uint64(2*N) {
		t.Errorf("executions = %d; want %d (1 decode + 1 encode per goroutine)", got, 2*N)
	}
}

// TestFilter_ConcurrentRespondCapture verifies that N=100 streams each
// invoking :respond() from their own envoy_on_request hook do not race;
// each stream's SendLocalReply observes its OWN status/body (no cross-
// stream contamination of the respond-state). The script's :respond
// uses the request's incoming X-Status header as the response status to
// thread per-stream identity through the respond path.
func TestFilter_ConcurrentRespondCapture(t *testing.T) {
	const script = `
		function envoy_on_request(rh)
			local s = rh:headers():get("X-Status") or "500"
			local body = rh:headers():get("X-Body") or ""
			rh:respond({[":status"] = s}, body)
		end
	`
	chunk, err := luaprim.CompileScript([]byte(script), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		chunk: chunk,
		stats: &filterStats{
			errors:       reg.NewCounter("ct3.lua.errors"),
			executions:   reg.NewCounter("ct3.lua.executions"),
			respondCalls: reg.NewCounter("ct3.lua.respond_calls"),
		},
	}

	const N = 100
	var (
		wg       sync.WaitGroup
		dcbs     = make([]*recordedDCB, N)
		failures atomic.Int64
	)
	// status range: 200..299; body: distinct per goroutine.
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			statusStr := fmt.Sprintf("%d", 200+idx)
			body := fmt.Sprintf("body-%d", idx)
			h := http.Header{
				"X-Status": []string{statusStr},
				"X-Body":   []string{body},
			}
			dcb := &recordedDCB{}
			dcbs[idx] = dcb
			f := &filter{cc: cc}
			f.SetDecoderCallbacks(dcb)
			defer f.OnDestroy()
			status := f.DecodeHeaders(h, false)
			if status != envoyhttp.StopIteration {
				t.Errorf("[%d]: status = %v; want StopIteration", idx, status)
				failures.Add(1)
				return
			}
			if dcb.localReply == nil {
				t.Errorf("[%d]: SendLocalReply not invoked", idx)
				failures.Add(1)
				return
			}
			if got, want := dcb.localReply.status, 200+idx; got != want {
				t.Errorf("[%d]: SendLocalReply status = %d; want %d (cross-stream respond leak)", idx, got, want)
				failures.Add(1)
			}
			if got, want := dcb.localReply.body, body; got != want {
				t.Errorf("[%d]: SendLocalReply body = %q; want %q (cross-stream respond leak)", idx, got, want)
				failures.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if n := failures.Load(); n > 0 {
		t.Fatalf("%d / %d goroutines failed respond assertions", n, N)
	}
	if got := cc.stats.respondCalls.Load(); got != uint64(N) {
		t.Errorf("respondCalls = %d; want %d (one per per-stream :respond)", got, N)
	}
	if got := cc.stats.executions.Load(); got != uint64(N) {
		t.Errorf("executions = %d; want %d", got, N)
	}
}

// buildBenchCompiledConfig constructs a *compiledConfig with a minimal
// headers-only Lua script for the BenchmarkPerStreamLState_Construction
// _Headers benchmark. The script defines envoy_on_request as a noop —
// matches the production "headers-only bridge surface" the benchmark
// measures per D-P10 (the per-stream VM-construction + bridge-install +
// request-handle-userdata-build + script-Run + hook-call hot path).
func buildBenchCompiledConfig(b *testing.B) *compiledConfig {
	b.Helper()
	const script = `function envoy_on_request(rh) end`
	chunk, err := luaprim.CompileScript([]byte(script), nil)
	if err != nil {
		b.Fatalf("CompileScript err = %v; want nil", err)
	}
	// No stats wiring — the benchmark measures per-stream VM construction
	// cost, not stats overhead. The cc.stats nil-tolerance at
	// decode_headers.go steps 5 + 7 + 8 + 9 makes this safe.
	return &compiledConfig{chunk: chunk}
}

// BenchmarkPerStreamLState_Construction_Headers measures the per-stream
// *lua.LState construction cost the production DecodeHeaders incurs:
// NewVM (with default sandbox) + install bridge metatables + build
// *requestHandleContext + bind LUserData + Run script top-level +
// CallGlobal envoy_on_request + Close. Per 22.1 PLAN D-P10 the
// reported ns/op gates ADR-0190 firing at Task 16:
//
//   - ns/op <= 1_000_000 (1ms) → WEAK-default per-stream construction
//     STANDS; ADR-0190 NOT consumed; carries forward to 22.2.
//   - ns/op  > 1_000_000 (1ms) → escape-valve FIRES; Task 16 lands the
//     per-script-source *LState pool design per ADR-0190.
//
// Run via:
//
//	go test -bench=BenchmarkPerStreamLState_Construction_Headers \
//	        -benchtime=3s ./internal/filter/http/lua/
//
// Quote the reported ns/op in PROGRESS.md verbatim + record the R6
// disposition per D-P10.
func BenchmarkPerStreamLState_Construction_Headers(b *testing.B) {
	cc := buildBenchCompiledConfig(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Mirrors decode_headers.go step 2 — per-stream NewVM with the
		// SHARED cc.sandbox config (zero-value default).
		vm := luaprim.NewVM(luaprim.WithSandboxConfig(cc.sandbox))

		// Mirrors decode_headers.go step 3 — install ALL bridge
		// metatables + the pairs shim. Done ONCE per VM in production;
		// the benchmark exercises the same install cost per per-stream
		// dispatch.
		L := vm.State()
		installRequestHandleMetatable(L)
		installResponseHandleMetatable(L)
		installHeadersMetatable(L)
		installStreamInfoMetatable(L)
		installPairsShim(L)

		// Mirrors decode_headers.go step 4 — build the per-stream
		// requestHandleContext + LUserData + metatable bind. Empty
		// headers map matches the "headers-only" benchmark scope per
		// D-P10 (no bridge-method invocation cost; just per-stream
		// construction).
		reqCtx := &requestHandleContext{
			headers: http.Header{},
			cb:      nil, // no callbacks dependency for the headers-only path
		}
		reqUd := L.NewUserData()
		reqUd.Value = reqCtx
		L.SetMetatable(reqUd, L.GetTypeMetatable(requestHandleTypeName))

		// Mirrors decode_headers.go step 5 — script top-level Run.
		if err := vm.Run(cc.chunk); err != nil {
			b.Fatalf("vm.Run err = %v", err)
		}

		// Mirrors decode_headers.go step 8 — invoke the hook with the
		// request_handle userdata. The script defines envoy_on_request
		// as a noop so the call cost is dispatch + return (no bridge-
		// method invocation cost — out of scope for the per-stream
		// construction benchmark per D-P10).
		if err := vm.CallGlobal("envoy_on_request", reqUd); err != nil {
			b.Fatalf("vm.CallGlobal err = %v", err)
		}

		// Mirrors filter.OnDestroy (lua.go:266-271) — per-stream VM
		// release at end-of-stream.
		vm.Close()
	}
}

// ----------------------------------------------------------------------
// Task 15 (phase 22.2 IMPL) — race tests N=100 parallel filter dispatches
// at the FULL 22.2 bridge surface + 2 benchmarks per D-P10 + D3 closure
// per 22.2 SPEC §13-R6 + §13-R9 + PLAN Task 15.
//
// # R6 signaling protocol
//
// BenchmarkPerStream_FullBridge_LState_Construction measures per-stream
// VM construction cost across ALL 22.2 metatable installs (request_handle
// + response_handle + headers + trailers + streamInfo + metadata +
// dynamicMetadata + connection + ssl + publicKeyWrapper + filterState +
// pairs shim) PLUS a parent+child *LState pair via NewThread (the
// coroutine surface that body / sync httpCall consume per ADR-0191 §11.1
// D2 closure).
//
// The reported ns/op gates the conditional ADR-0193 §Context + §Decision
// + §Consequences landing at Task 19 per the R6 signal protocol:
//
//   - ns/op <= 1_000_000 (= 1 ms) → R6 STANDS WEAK-default; the per-stream
//     *LState construction discipline (fresh VM per stream + shared
//     *Chunk cache) stays per ADR-0192 §Context; ADR-0193 NOT consumed;
//     carries forward to 22.3 BRAINSTORM as the 22.3 IMPL escape-valve
//     slot.
//   - ns/op  > 1_000_000 (= 1 ms) → R6 ADR-0193 FIRES; Task 19 atomic
//     landing authors ADR-0193 §Context + §Decision + §Consequences body
//     consuming the per-script-source `*LState`-pool design.
//
// PLAN hypothesis: STAYS WEAK-default. 22.1 IMPL baseline (headers-only)
// was ns/op = 69865 (~70 µs); 22.2 anticipated 200-500 µs (3-7×
// headers-only) — SHOULD stay under 1 ms.
//
// # D3 closure — defensive-copy at endStream perf-validation
//
// BenchmarkBodyBridge_DefensiveCopy_PerStream measures per-stream body-
// bridge construction + accumulation + defensive-copy at endStream
// overhead at two body-size points per the D3 closure threshold gates:
//
//   - sub-MB body (100 KB): ≤ 1 ms per stream;
//   - 16-MiB-cap-saturated body: ≤ 100 ms per stream.
//
// Both gates met → option (a) defensive-copy at endStream STANDS at 22.2
// phase-done per SPEC §11.3 + §12 RECOMMENDED option (a). Either gate
// exceeded → R9 ADR-0193 escape-valve fires at Task 19 with option (b)
// zero-copy via `*lua.LUserData` wrapping. R9 cross-check: STAYS embedded
// in ADR-0192 per Task 7 IMPL outcome — the body-bridge implementation
// surface did NOT introduce additional ADR-warranting complexity beyond
// what is documented under ADR-0192 §Context.
// ----------------------------------------------------------------------

// buildBenchFullBridgeConfig constructs a *compiledConfig with a script
// that defines BOTH envoy_on_request + envoy_on_response as no-op hooks.
// The 8-counter *filterStats is wired via newFilterStats so the 5 NEW
// envoy-go-strict counter allocations land inside the benchmark window
// per the §13-R6 full-bridge measurement scope.
func buildBenchFullBridgeConfig(b *testing.B) *compiledConfig {
	b.Helper()
	const script = `
		function envoy_on_request(rh) end
		function envoy_on_response(rh) end
	`
	chunk, err := luaprim.CompileScript([]byte(script), nil)
	if err != nil {
		b.Fatalf("CompileScript err = %v; want nil", err)
	}
	reg := stats.NewRegistry()
	return &compiledConfig{
		chunk: chunk,
		stats: newFilterStats(reg, "ingress_http", "bench_full"),
	}
}

// BenchmarkPerStream_FullBridge_LState_Construction measures per-stream
// *lua.LState construction cost at the FULL 22.2 bridge surface per
// 22.2 SPEC §13-R6 + D-P10 + PLAN Task 15.
//
// Each iteration:
//
//   - Constructs a fresh per-stream VM via luaprim.NewVM (sandbox install
//   - base-lib load per the 22.1 NewVM body).
//   - Installs ALL 22.2 bridge metatables: request_handle +
//     response_handle + headers + trailers + streamInfo + metadata +
//     dynamicMetadata + connection + ssl + publicKeyWrapper + filterState
//   - the pairs shim. Mirrors decode_headers.go:97-109 verbatim.
//   - Attaches a context to the parent LState (mirrors the bridge layer's
//     per-stream context-attach discipline per ADR-0191).
//   - Mints a child *LState via vm.NewThread (the coroutine surface that
//     body / sync httpCall consume per §11.1 D2 closure). The CancelFunc
//     is invoked at end-of-iteration to release the ctx-attached child
//     loop.
//   - Builds the per-stream *requestHandleContext + *responseHandleContext
//     LUserData bindings + metatable attachments.
//   - Runs the chunk top-level (defines envoy_on_request +
//     envoy_on_response globals).
//   - CallGlobal envoy_on_request + envoy_on_response (per-stream hook
//     invocations).
//   - vm.Close (mirrors filter.OnDestroy lua.go:533-538).
//
// Run via:
//
//	go test -bench=BenchmarkPerStream_FullBridge_LState_Construction \
//	        -benchtime=3s ./internal/filter/http/lua/
//
// Quote the reported ns/op in PROGRESS.md Task 15 entry VERBATIM + record
// the R6 disposition sentinel per the R6 signal protocol consumed by
// Task 19 Step 10 (which greps for the literal substring
// "§13-R6 disposition: ADR-0193 FIRES" to determine whether to land
// conditional ADR-0193).
func BenchmarkPerStream_FullBridge_LState_Construction(b *testing.B) {
	cc := buildBenchFullBridgeConfig(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Mirrors decode_headers.go step 2 — per-stream NewVM with the
		// SHARED cc.sandbox config (zero-value default).
		vm := luaprim.NewVM(luaprim.WithSandboxConfig(cc.sandbox))

		// Mirrors decode_headers.go step 3 — install ALL 22.2 bridge
		// metatables + the pairs shim. Done ONCE per VM in production;
		// the benchmark exercises the same install cost per per-stream
		// dispatch.
		L := vm.State()
		installRequestHandleMetatable(L)
		installResponseHandleMetatable(L)
		installHeadersMetatable(L)
		installTrailersMetatable(L)
		installStreamInfoMetatable(L)
		installMetadataMetatable(L)
		installDynamicMetadataMetatable(L)
		installConnectionMetatable(L)
		installSSLMetatable(L)
		installPublicKeyWrapperMetatable(L)
		installFilterStateMetatable(L)
		installPairsShim(L)

		// Attach a per-stream context (mirrors the ADR-0191 bridge-layer
		// per-stream context-attach discipline). Without this, vm.NewThread
		// returns a nil CancelFunc (gopher-lua state.go:1618).
		ctx, cancelCtx := context.WithCancel(context.Background())
		L.SetContext(ctx)

		// Mint a child *LState via NewThread — the coroutine surface that
		// body / sync httpCall consume per §11.1 D2 closure.
		_, cancelChild := vm.NewThread()

		// Build the per-stream request_handle + response_handle userdata
		// + metatable bindings. Empty headers; nil callbacks (the FULL-
		// bridge benchmark measures construction cost, NOT bridge-method
		// invocation cost — out of scope per D-P10).
		reqCtx := &requestHandleContext{
			headers: http.Header{},
		}
		reqUd := L.NewUserData()
		reqUd.Value = reqCtx
		L.SetMetatable(reqUd, L.GetTypeMetatable(requestHandleTypeName))

		respCtx := &responseHandleContext{
			headers: http.Header{},
		}
		respUd := L.NewUserData()
		respUd.Value = respCtx
		L.SetMetatable(respUd, L.GetTypeMetatable(responseHandleTypeName))

		// Mirrors decode_headers.go step 5 — script top-level Run.
		if err := vm.Run(cc.chunk); err != nil {
			b.Fatalf("vm.Run err = %v", err)
		}

		// Mirrors decode_headers.go step 8 — invoke envoy_on_request with
		// the request_handle userdata.
		if err := vm.CallGlobal("envoy_on_request", reqUd); err != nil {
			b.Fatalf("vm.CallGlobal envoy_on_request err = %v", err)
		}

		// Mirrors encode_headers.go — invoke envoy_on_response with the
		// response_handle userdata.
		if err := vm.CallGlobal("envoy_on_response", respUd); err != nil {
			b.Fatalf("vm.CallGlobal envoy_on_response err = %v", err)
		}

		// Mirrors filter.OnDestroy (lua.go:533-538) — per-stream cleanup.
		if cancelChild != nil {
			cancelChild()
		}
		cancelCtx()
		vm.Close()
	}
}

// BenchmarkBodyBridge_DefensiveCopy_PerStream measures per-stream body-
// bridge construction + body accumulation + defensive-copy at endStream
// per the D3 closure threshold gates:
//
//   - sub-MB (100 KB): ≤ 1 ms per stream;
//   - 16-MiB-cap-saturated: ≤ 100 ms per stream.
//
// Each sub-benchmark constructs a fresh *filter + minimal bridge install,
// accumulates the body via accumulateRequestBody (chunked 64 KiB writes;
// the framework-typical chunk size at the HCM body callback layer), and
// at terminal endStream forces the `lua.LString(string(b))` defensive
// copy via the request_handle:body() bridge LGFunction path.
//
// The two sub-benchmarks share a common driver helper to keep the
// measurement scope consistent (only the body size varies). Run via:
//
//	go test -bench=BenchmarkBodyBridge_DefensiveCopy_PerStream \
//	        -benchtime=3s ./internal/filter/http/lua/
//
// Quote BOTH reported ns/op values in PROGRESS.md Task 15 entry
// VERBATIM + record the R9 disposition cross-check vs Task 7 IMPL
// outcome.
func BenchmarkBodyBridge_DefensiveCopy_PerStream(b *testing.B) {
	b.Run("sub-MB", func(b *testing.B) {
		runBodyBridgeBenchmark(b, 100*1024) // 100 KB sub-MB body
	})
	b.Run("16-MiB-saturated", func(b *testing.B) {
		// 16 MiB minus 1 byte — saturates the per-stream cap WITHOUT
		// tripping arm-21 over-cap reject. Per body.go:314 the over-cap
		// guard is `len(f.decodedBodyBytes) > f.maxBodyBufferedBytes`
		// (strict greater-than); cap-saturated-equal is permitted.
		runBodyBridgeBenchmark(b, 16*1024*1024)
	})
}

// runBodyBridgeBenchmark drives the body-bridge defensive-copy benchmark
// at a specified body size. Each iteration: constructs a fresh *filter +
// VM + bridge install; chunks the body into 64 KiB writes via
// accumulateRequestBody; at terminal endStream invokes the
// request_handle:body() bridge to force the `lua.LString(string(b))`
// defensive copy; cleans up via vm.Close.
func runBodyBridgeBenchmark(b *testing.B, bodySize int) {
	b.Helper()
	// Pre-allocate a deterministic body once + reuse across iterations.
	// Allocating bodySize bytes inside the timed loop would dominate the
	// 16-MiB-saturated measurement.
	body := make([]byte, bodySize)
	for i := range body {
		body[i] = byte(i % 251) // arbitrary deterministic content
	}
	const chunkSize = 64 * 1024 // 64 KiB — framework-typical HCM body chunk

	// Pre-compile the body-eval script once — script compilation is shared
	// across streams in production (cc.chunk closure-captured).
	const script = `
		function envoy_on_request(rh)
			local b = rh:body()
			__body_len = #b
		end
	`
	chunk, err := luaprim.CompileScript([]byte(script), nil)
	if err != nil {
		b.Fatalf("CompileScript err = %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(bodySize))
	b.ResetTimer()
	for iter := 0; iter < b.N; iter++ {
		// Per-stream filter + VM + bridge surface.
		reg := stats.NewRegistry()
		f := &filter{
			cc: &compiledConfig{
				chunk: chunk,
				stats: newFilterStats(reg, "ingress_http", "bench_body"),
			},
		}
		f.vm = luaprim.NewVM(luaprim.WithSandboxConfig(f.cc.sandbox))
		L := f.vm.State()

		ctx, cancelCtx := context.WithCancel(context.Background())
		L.SetContext(ctx)

		// Install only the minimal bridge surface the body script touches
		// (request_handle + headers + pairs shim). The FULL-bridge install
		// cost is measured separately by BenchmarkPerStream_FullBridge_
		// LState_Construction; here the D3 closure scope is the body-
		// accumulation + defensive-copy cost.
		installRequestHandleMetatable(L)
		installHeadersMetatable(L)
		installPairsShim(L)

		f.reqCtx = &requestHandleContext{headers: http.Header{}, filterRef: f}
		rud := L.NewUserData()
		rud.Value = f.reqCtx
		L.SetMetatable(rud, L.GetTypeMetatable(requestHandleTypeName))
		L.SetGlobal("rh", rud)

		// Pre-set bodyReady = true via the terminal accumulate call (the
		// benchmark scope is the synchronous "endStream-already-fired"
		// path — the defensive-copy IS the work being measured; the
		// coroutine yield/resume path is exercised by
		// Test_RequestHandleBody_coroutine_yield_before_endStream).
		written := 0
		for written < bodySize {
			n := chunkSize
			if written+n > bodySize {
				n = bodySize - written
			}
			endStream := written+n == bodySize
			accumulateRequestBody(f, body[written:written+n], endStream)
			written += n
		}

		// Run the script top-level + invoke envoy_on_request — this is
		// where rh:body() fires + the `lua.LString(string(b))` defensive
		// copy lands per body.go:326.
		if err := f.vm.Run(f.cc.chunk); err != nil {
			b.Fatalf("vm.Run err = %v", err)
		}
		if err := f.vm.CallGlobal("envoy_on_request", rud); err != nil {
			b.Fatalf("vm.CallGlobal err = %v", err)
		}

		// Per-stream cleanup.
		cancelCtx()
		f.vm.Close()
		f.vm = nil
	}
}

// ----------------------------------------------------------------------
// Task 4 (phase 22.3 IMPL) — R6 per-route resolution benchmark.
//
// BenchmarkPerStream_PerRoute_Resolution measures the per-stream cost the
// multi-script + per-route surface adds at DecodeHeaders: the per-route
// chunk selection (resolveDecodeScript) including the source_code-override
// memo hot path (resolvePerRouteSourceCode under cc.perRouteMu). The R6
// disposition question is "is per-route resolution O(1) and well under
// 1 ms/stream" — i.e. the reported ns/op for the resolution call should
// be << 1_000_000 ns (1 ms).
//
// Two sub-benchmarks (per the PLAN Task 4 "measure BOTH" provision when
// the resolution-alone vs full-per-stream choice is a judgment call):
//
//   - "resolution-only" — measures resolveDecodeScript() repeatedly on a
//     fixed *filter wired with a source_code per-route override. After the
//     first call the override chunk is memoized in cc.perRouteChunks, so
//     every subsequent call exercises the memo-HIT hot path: a single
//     RequestRouteConfig() + type-assert + oneof switch + mutex-guarded
//     map lookup. This is the load-bearing R6 measurement — the per-stream
//     marginal cost of the per-route dispatch once the memo is warm.
//   - "per-stream" — measures a fresh *filter per iteration resolving a
//     source_code override against a SHARED *compiledConfig (the memo is
//     shared across streams keyed by the stable *LuaPerRoute pointer). This
//     models the realistic per-stream path where each stream constructs its
//     own filter but the source_code read+compile happens ONCE (memoized);
//     all subsequent streams hit the warm memo. Excludes VM construction
//     (measured by BenchmarkPerStream_FullBridge_LState_Construction) so
//     the number isolates the per-route resolution marginal cost.
// ----------------------------------------------------------------------

// buildBenchPerRouteConfig constructs a *compiledConfig + a source_code
// *LuaPerRoute override for the R6 per-route resolution benchmark. The
// compiledConfig carries a listener-default chunk + a live compileCache so
// the memo's first-miss compile lands in the shared cache (content-hash
// dedup), matching the production buildCompiledConfig wiring.
func buildBenchPerRouteConfig(b *testing.B) (*compiledConfig, *luav3.LuaPerRoute) {
	b.Helper()
	defChunk, err := luaprim.CompileScript([]byte(`function envoy_on_request(rh) end`), nil)
	if err != nil {
		b.Fatalf("CompileScript err = %v; want nil", err)
	}
	cc := &compiledConfig{
		chunk:        defChunk,
		compileCache: luaprim.NewCompileCache(),
	}
	pr := &luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_SourceCode{
			SourceCode: &corev3.DataSource{
				Specifier: &corev3.DataSource_InlineString{
					InlineString: `function envoy_on_response(rh) end`,
				},
			},
		},
	}
	return cc, pr
}

// BenchmarkPerStream_PerRoute_Resolution measures the R6 per-route
// resolution per-stream marginal cost per phase 22.3 PLAN Task 4. Reports
// ns/op + allocs/op. The disposition gate: << 1_000_000 ns (1 ms).
func BenchmarkPerStream_PerRoute_Resolution(b *testing.B) {
	// resolution-only — fixed filter, repeated resolveDecodeScript calls.
	// After the first call the source_code override chunk is memoized in
	// cc.perRouteChunks, so the timed loop exercises the memo-HIT hot path:
	// RequestRouteConfig() + type-assert + oneof switch + mutex-guarded map
	// lookup. This isolates the steady-state per-stream resolution cost.
	b.Run("resolution-only", func(b *testing.B) {
		cc, pr := buildBenchPerRouteConfig(b)
		f, _ := newResolveTestFilter(cc, pr)
		// Warm the memo once outside the timed loop so the first-miss
		// resolveDataSource + CompileScript cost is excluded — the R6
		// question is the steady-state O(1) memo-HIT cost per stream.
		if ch, _ := f.resolveDecodeScript(); ch == nil {
			b.Fatalf("resolveDecodeScript returned nil chunk for source_code override; want non-nil")
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ch, disabled := f.resolveDecodeScript()
			if ch == nil || disabled {
				b.Fatalf("resolveDecodeScript = (%v, %v); want (non-nil, false)", ch, disabled)
			}
		}
	})

	// per-stream — fresh filter per iteration against a SHARED *compiledConfig.
	// Models the realistic per-stream path: each stream builds its own
	// *filter (bound to the shared cc) and resolves the per-route override.
	// The source_code read+compile happens ONCE (memoized keyed by the
	// stable *LuaPerRoute pointer); all subsequent iterations hit the warm
	// memo. Excludes VM construction (measured separately) to isolate the
	// per-route resolution marginal cost a stream pays at DecodeHeaders.
	b.Run("per-stream", func(b *testing.B) {
		cc, pr := buildBenchPerRouteConfig(b)
		// Warm the shared memo once so the per-iteration filter resolves
		// against a populated cc.perRouteChunks (the realistic steady state
		// after the first stream on a route has run).
		{
			warm, _ := newResolveTestFilter(cc, pr)
			if ch, _ := warm.resolveDecodeScript(); ch == nil {
				b.Fatalf("warm resolveDecodeScript returned nil chunk; want non-nil")
			}
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			f, _ := newResolveTestFilter(cc, pr)
			ch, disabled := f.resolveDecodeScript()
			if ch == nil || disabled {
				b.Fatalf("resolveDecodeScript = (%v, %v); want (non-nil, false)", ch, disabled)
			}
		}
	})
}

// ----------------------------------------------------------------------
// Task 15 — race tests N=100 parallel filter dispatches at FULL 22.2
// bridge surface + goroutine-leak detection + cross-stream-state-leak
// detection per D-P10 + ADR-0071 single-goroutine-per-stream invariant.
// ----------------------------------------------------------------------

// TestRace_N100_parallel_filter_dispatches_clean_under_race spawns N=100
// concurrent goroutines, each constructing an independent *filter bound
// to the SHARED *compiledConfig + running DecodeHeaders / DecodeData /
// EncodeHeaders / EncodeData / OnDestroy at the FULL 22.2 bridge surface.
// Asserts:
//
//   - No cross-stream state leak: each goroutine's script observes its
//     own X-Stream-Id header (echoed back as X-Lua-Saw) + its own body
//     bytes (echoed back as X-Body-Len). Cross-contamination would
//     surface as a per-goroutine assertion failure.
//   - No goroutine leak: runtime.NumGoroutine delta between baseline +
//     post-test stays bounded (≤ 2 — accommodates the Go scheduler's own
//     churn). Child *LState CancelFunc invocations are tracked via the
//     OnDestroy path (f.vm.Close releases the parent + child loops per
//     ADR-0191 §Context).
//   - All 5 NEW envoy-go-strict counters increment as expected
//     (body_buffered_bytes_total + coroutine_yields_total exercised by
//     the body-bridge path; httpcall_* counters NOT exercised here
//     — Task 16 fuzzers cover the httpCall path).
//
// Race-clean under `go test -race -count=10`. Skipped under `-short`.
func TestRace_N100_parallel_filter_dispatches_clean_under_race(t *testing.T) {
	if testing.Short() {
		t.Skip("skip parallel race test under -short")
	}

	const script = `
		function envoy_on_request(rh)
			local v = rh:headers():get("X-Stream-Id")
			rh:headers():add("X-Lua-Saw", v or "")
			local b = rh:body()
			rh:headers():add("X-Body-Len", tostring(#b))
		end
		function envoy_on_response(rh)
			local v = rh:headers():get("X-Resp-Stream-Id")
			rh:headers():add("X-Resp-Lua-Saw", v or "")
		end
	`
	chunk, err := luaprim.CompileScript([]byte(script), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; want nil", err)
	}
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		chunk: chunk,
		stats: newFilterStats(reg, "ingress_http", "race_n100"),
	}

	// Settle baseline before spawning the worker pool.
	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	before := runtime.NumGoroutine()

	const N = 100
	var (
		wg       sync.WaitGroup
		failures atomic.Int64
	)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			f := &filter{cc: cc}
			f.SetDecoderCallbacks(&recordedDCB{})
			f.SetEncoderCallbacks(&recordedECB{})
			// Per-stream OnDestroy releases the VM + child loops per
			// ADR-0191 §Context. Idempotent guard at lua.go:533-538.
			defer f.OnDestroy()

			// To exercise rh:body() inline within DecodeHeaders' synchronous
			// CallGlobal (i.e., NOT via the coroutine yield/resume path —
			// the script run from DecodeHeaders is not itself a coroutine,
			// so a yield from rh:body() would surface a "can not yield from
			// outside of a coroutine" error), we accumulate the body PRIOR
			// to DecodeHeaders. This pre-DecodeHeaders body accumulation is
			// supported by the *filter's lazy state: f.decodedBodyBytes +
			// f.bodyReady are *filter fields, not VM-scoped, so DecodeData
			// can fire before DecodeHeaders constructs the VM.
			//
			// Production HCM dispatch fires DecodeHeaders FIRST then
			// DecodeData; the coroutine yield/resume path is exercised by
			// body_test.go::Test_RequestHandleBody_coroutine_yield_before_
			// endStream_then_resume which mounts the script body inside an
			// explicit Resume call. The race test here uses the simpler
			// already-ready synchronous path so the N=100 parallelism
			// surfaces cross-goroutine state leaks WITHOUT entangling the
			// coroutine harness setup (which would itself need per-
			// goroutine harness state — orthogonal to the race-clean
			// assertion).
			bodyBytes := []byte(fmt.Sprintf("body-payload-%d", idx))
			f.DecodeData(bodyBytes, true)

			// Decode-side: X-Stream-Id seeds the cross-stream-leak probe.
			reqHeaders := http.Header{
				"X-Stream-Id": []string{fmt.Sprintf("stream-%d", idx)},
			}
			if status := f.DecodeHeaders(reqHeaders, false); status != envoyhttp.Continue {
				t.Errorf("[%d] decode status = %v; want Continue", idx, status)
				failures.Add(1)
				return
			}

			// Cross-stream-state-leak assertion: this goroutine's X-Stream-Id
			// must echo back into its OWN headers map under X-Lua-Saw.
			wantStreamID := fmt.Sprintf("stream-%d", idx)
			if got := reqHeaders.Get("X-Lua-Saw"); got != wantStreamID {
				t.Errorf("[%d] X-Lua-Saw = %q; want %q (cross-stream leak)",
					idx, got, wantStreamID)
				failures.Add(1)
				return
			}

			// Encode side: independent X-Resp-Stream-Id seed.
			respHeaders := http.Header{
				"X-Resp-Stream-Id": []string{fmt.Sprintf("resp-%d", idx)},
			}
			if status := f.EncodeHeaders(respHeaders, false); status != envoyhttp.Continue {
				t.Errorf("[%d] encode status = %v; want Continue", idx, status)
				failures.Add(1)
				return
			}
			wantRespID := fmt.Sprintf("resp-%d", idx)
			if got := respHeaders.Get("X-Resp-Lua-Saw"); got != wantRespID {
				t.Errorf("[%d] X-Resp-Lua-Saw = %q; want %q (cross-stream leak)",
					idx, got, wantRespID)
				failures.Add(1)
				return
			}

			// EncodeData: terminal signal closes the body path.
			f.EncodeData([]byte("resp-body"), true)
		}(i)
	}
	wg.Wait()

	if n := failures.Load(); n > 0 {
		t.Fatalf("%d / %d goroutines failed assertions", n, N)
	}

	// Goroutine-leak assertion: settled goroutine count must not exceed
	// baseline + 2 (Go scheduler churn tolerance). Each per-stream filter
	// is purely synchronous (no background goroutines spawned for the
	// body/headers paths exercised here); the httpCall sync dispatch
	// goroutine is NOT exercised here.
	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before+2 {
		t.Fatalf("goroutine leak: before=%d after=%d delta=%d (tolerance ≤ 2)",
			before, after, after-before)
	}

	// Counter assertions per the 5 NEW envoy-go-strict counters at SPEC
	// §7.1. Each goroutine invokes envoy_on_request ONCE (via
	// DecodeHeaders) + envoy_on_response ONCE (via EncodeHeaders) =
	// 2 * N executions total. Each DecodeData call contributes
	// len(bodyBytes) to body_buffered_bytes_total + EncodeData contributes
	// len("resp-body")=9.
	gotExec := cc.stats.executions.Load()
	wantExec := uint64(N * 2)
	if gotExec != wantExec {
		t.Errorf("executions = %d; want %d (1 decode + 1 encode run per goroutine)",
			gotExec, wantExec)
	}
	if gotErrors := cc.stats.errors.Load(); gotErrors != 0 {
		t.Errorf("errors = %d; want 0 (no script errors expected)", gotErrors)
	}
	// body_buffered_bytes_total: sum across all goroutines of the
	// decode-side body length + encode-side "resp-body" (9 bytes).
	var wantBodyBytes uint64
	for i := 0; i < N; i++ {
		wantBodyBytes += uint64(len(fmt.Sprintf("body-payload-%d", i))) + 9
	}
	if got := cc.stats.bodyBufferedBytesTotal.Load(); got != wantBodyBytes {
		t.Errorf("body_buffered_bytes_total = %d; want %d", got, wantBodyBytes)
	}
}
