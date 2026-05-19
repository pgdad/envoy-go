package lua

// lua_test.go — Task 1 skeleton + Task 9 decode/encode dispatch
// integration tests + Task 10 factory-body + stats-registration tests.
// Task 1: TypeURL byte-pin. Task 9: DecodeHeaders + EncodeHeaders +
// OnDestroy + SendLocalReply test-double end-to-end integration. Task
// 10: New full-body wiring (happy-path + nil typed_config + 3-counter
// HCM-rooted registration + empty-prefix consecutive-dot + cardinality
// + statName* byte-exact constants).

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	luav3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

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

// TestStatNames_TableDriven asserts all 3 stat name constants in one
// table-driven row per the §6.6 dual-layer guard precedent (mirrors
// adaptive_concurrency::TestStatNames_Equal_* shape).
func TestStatNames_TableDriven(t *testing.T) {
	tests := []struct {
		gotConst, want string
	}{
		{statNameErrors, "errors"},
		{statNameExecutions, "executions"},
		{statNameRespondCalls, "respond_calls"},
	}
	for _, tc := range tests {
		if tc.gotConst != tc.want {
			t.Errorf("stat name constant = %q; want %q", tc.gotConst, tc.want)
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

// TestNewFilterStats_RegistersThreeCounters_HCMRootedTemplate verifies
// that newFilterStats(reg, "ingress_http", "my_prefix") registers
// exactly the 3 byte-exact wire names under the HCM-rooted template
// per parent §7.2 + AMEND-2.
func TestNewFilterStats_RegistersThreeCounters_HCMRootedTemplate(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_http", "my_prefix")
	if fs == nil {
		t.Fatal("newFilterStats returned nil; want non-nil")
	}
	want := map[string]bool{
		"http.ingress_http.lua.my_prefix.errors":        true,
		"http.ingress_http.lua.my_prefix.executions":    true,
		"http.ingress_http.lua.my_prefix.respond_calls": true,
	}
	got := walkRegistry(reg)
	if len(got) != 3 {
		t.Fatalf("registered count = %d; want 3 (got names: %v)", len(got), got)
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
	if fs.errors == nil || fs.executions == nil || fs.respondCalls == nil {
		t.Errorf("filterStats has nil counter field: errors=%v executions=%v respondCalls=%v",
			fs.errors, fs.executions, fs.respondCalls)
	}
}

// TestNewFilterStats_CardinalityAssertion asserts exactly 3 counters
// are registered per filter instance per Task 10 acceptance criteria.
// Detects regressions where a future maintainer might add a 4th stat
// without updating BEHAVIOR_CONTRACT.md §13.6 + the parent §7 roster.
func TestNewFilterStats_CardinalityAssertion(t *testing.T) {
	reg := stats.NewRegistry()
	_ = newFilterStats(reg, "h", "c")
	got := walkRegistry(reg)
	if len(got) != 3 {
		t.Fatalf("cardinality = %d; want exactly 3 (names: %v)", len(got), got)
	}
}

// TestNewFilterStats_EmptyConfigStatPrefix_ConsecutiveDot verifies the
// AMEND-2 consecutive-dot literal: when `Lua.stat_prefix` is empty the
// registered wire names contain `lua..` (two consecutive dots). Mirrors
// the phase-14 compressor empty-`<library>` precedent at
// BEHAVIOR_CONTRACT.md §line 243.
func TestNewFilterStats_EmptyConfigStatPrefix_ConsecutiveDot(t *testing.T) {
	reg := stats.NewRegistry()
	_ = newFilterStats(reg, "ingress_http", "")
	want := map[string]bool{
		"http.ingress_http.lua..errors":        true,
		"http.ingress_http.lua..executions":    true,
		"http.ingress_http.lua..respond_calls": true,
	}
	got := walkRegistry(reg)
	if len(got) != 3 {
		t.Fatalf("registered count = %d; want 3 (got: %v)", len(got), got)
	}
	for _, n := range got {
		if !want[n] {
			t.Errorf("unexpected registered name %q (want consecutive-dot form)", n)
		}
	}
}

// TestNewFilterStats_EmptyHcmAndConfig_DoubleConsecutiveDot verifies
// the AMEND-2 corner case: both ctx.StatPrefix AND Lua.StatPrefix empty
// → `http..lua..<stat>` (two consecutive-dot pairs). The registry name
// regex permits interior consecutive dots per
// internal/stats/registry.go::nameRE; this test pins the operational-
// degenerate-but-valid wire shape.
func TestNewFilterStats_EmptyHcmAndConfig_DoubleConsecutiveDot(t *testing.T) {
	reg := stats.NewRegistry()
	_ = newFilterStats(reg, "", "")
	want := map[string]bool{
		"http..lua..errors":        true,
		"http..lua..executions":    true,
		"http..lua..respond_calls": true,
	}
	got := walkRegistry(reg)
	if len(got) != 3 {
		t.Fatalf("registered count = %d; want 3 (got: %v)", len(got), got)
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
	if len(got) != 3 {
		t.Fatalf("registered count = %d; want 3 (got: %v)", len(got), got)
	}
	wantNames := map[string]bool{
		"http.ingress_http.lua.my_script.errors":        true,
		"http.ingress_http.lua.my_script.executions":    true,
		"http.ingress_http.lua.my_script.respond_calls": true,
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
		"http.ingress_http.lua..errors":        true,
		"http.ingress_http.lua..executions":    true,
		"http.ingress_http.lua..respond_calls": true,
	}
	if len(got) != 3 {
		t.Fatalf("registered count = %d; want 3 (got: %v)", len(got), got)
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

// TestRegisterPerRouteValidator_WiresArmEighteenRejection verifies the
// exported RegisterPerRouteValidator wires `validatePerRouteLua`
// (returning the arm-18 PARSE-REJECT) under the canonical filterName
// `envoy.filters.http.lua`. Pattern mirrors header_mutation +
// oauth2's `TestRegisterPerRouteValidator` regression precedent.
func TestRegisterPerRouteValidator_WiresArmEighteenRejection(t *testing.T) {
	fr := &fakeRegistry{}
	RegisterPerRouteValidator(fr)
	v, ok := fr.registered[filterName]
	if !ok {
		t.Fatalf("validator NOT registered under filterName %q (registered: %v)", filterName, fr.registered)
	}
	if v == nil {
		t.Fatal("registered validator is nil; want non-nil")
	}
	// Wired validator must return the arm-18 PARSE-REJECT byte-exact.
	err := v(&luav3.Lua{}) // any non-nil proto.Message; validator ignores body at 22.1.
	if err == nil {
		t.Fatal("validator returned nil err; want arm-18 PARSE-REJECT")
	}
	if err.Error() != parseRejectPerRouteDeferred {
		t.Fatalf("validator err = %q; want %q", err.Error(), parseRejectPerRouteDeferred)
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
