package header_mutation

import (
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	commonmutationrulesv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	headermutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

func mustAny(t *testing.T, m proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

func mkAppendOp(name, value string, action corev3.HeaderValueOption_HeaderAppendAction) *commonmutationrulesv3.HeaderMutation {
	return &commonmutationrulesv3.HeaderMutation{
		Action: &commonmutationrulesv3.HeaderMutation_Append{
			Append: &corev3.HeaderValueOption{
				Header:       &corev3.HeaderValue{Key: name, Value: value},
				AppendAction: action,
			},
		},
	}
}

func mkRemoveOp(name string) *commonmutationrulesv3.HeaderMutation {
	return &commonmutationrulesv3.HeaderMutation{
		Action: &commonmutationrulesv3.HeaderMutation_Remove{Remove: name},
	}
}

func TestNew_NilTC(t *testing.T) {
	_, err := New(nil, envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
	if err == nil {
		t.Fatal("expected error for nil tc; got nil")
	}
	if !strings.Contains(err.Error(), "typed_config required") {
		t.Errorf("error: got %v, want 'typed_config required'", err)
	}
}

func TestNew_MalformedTC(t *testing.T) {
	bad := &anypb.Any{TypeUrl: "type.googleapis.com/garbage", Value: []byte{0xff, 0xff, 0xff}}
	_, err := New(bad, envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
	if err == nil {
		t.Fatal("expected error for malformed tc; got nil")
	}
}

func TestNew_ProtectedHeader(t *testing.T) {
	cases := []struct {
		name       string
		headerName string
		side       string // "request" or "response"
	}{
		{"method-request", ":method", "request"},
		{"path-request", ":path", "request"},
		{"authority-request", ":authority", "request"},
		{"scheme-request", ":scheme", "request"},
		{"status-request", ":status", "request"},
		{"host-lower-request", "host", "request"},
		{"host-title-request", "Host", "request"},
		{"host-upper-request", "HOST", "request"},
		{"status-response", ":status", "response"},
		{"host-response", "host", "response"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mut := &headermutationv3.HeaderMutation{Mutations: &headermutationv3.Mutations{}}
			op := mkAppendOp(tc.headerName, "v", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)
			switch tc.side {
			case "request":
				mut.Mutations.RequestMutations = []*commonmutationrulesv3.HeaderMutation{op}
			case "response":
				mut.Mutations.ResponseMutations = []*commonmutationrulesv3.HeaderMutation{op}
			}
			_, err := New(mustAny(t, mut), envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
			if err == nil {
				t.Fatalf("%s: expected protected-header error; got nil", tc.headerName)
			}
			// Verbatim message format per ADR-0111 / SPEC §11.1 (f).
			wantPrefix := "header_mutation: "
			wantSuffix := " is :-prefixed or host; may not be modified"
			if !strings.Contains(err.Error(), wantPrefix) || !strings.Contains(err.Error(), wantSuffix) {
				t.Errorf("error: got %q, want '%s\"%s\"%s'", err, wantPrefix, tc.headerName, wantSuffix)
			}
		})
	}
}

func TestNew_ProtectedHeader_RemoveAlsoRejected(t *testing.T) {
	mut := &headermutationv3.HeaderMutation{
		Mutations: &headermutationv3.Mutations{
			RequestMutations: []*commonmutationrulesv3.HeaderMutation{mkRemoveOp(":path")},
		},
	}
	_, err := New(mustAny(t, mut), envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
	if err == nil {
		t.Fatal("expected error for Remove of :path; got nil")
	}
}

func TestNew_HappyPath_ListenerLevelOnly(t *testing.T) {
	mut := &headermutationv3.HeaderMutation{
		Mutations: &headermutationv3.Mutations{
			RequestMutations: []*commonmutationrulesv3.HeaderMutation{
				mkAppendOp("x-test", "v", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
				mkAppendOp("x-add", "v2", corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD),
				mkRemoveOp("user-agent"),
			},
			ResponseMutations: []*commonmutationrulesv3.HeaderMutation{
				mkAppendOp("x-resp", "rv", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
			},
		},
		MostSpecificHeaderMutationsWins: true,
	}
	factory, err := New(mustAny(t, mut), envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
	if err != nil {
		t.Fatalf("happy path: %v", err)
	}
	inst := factory()
	if inst.Decoder == nil || inst.Encoder == nil {
		t.Errorf("expected both Decoder and Encoder set; got %+v", inst)
	}
	if inst.Name != filterName {
		t.Errorf("Name: got %q, want %q", inst.Name, filterName)
	}
}

func TestRuntimeConfig_FieldExtraction(t *testing.T) {
	c := &headermutationv3.HeaderMutation{
		Mutations: &headermutationv3.Mutations{
			RequestMutations: []*commonmutationrulesv3.HeaderMutation{
				mkAppendOp("x-req", "rv", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
			},
			ResponseMutations: []*commonmutationrulesv3.HeaderMutation{
				mkRemoveOp("x-rm"),
			},
		},
		MostSpecificHeaderMutationsWins: true,
	}
	rc, err := buildRuntimeConfig(c)
	if err != nil {
		t.Fatalf("buildRuntimeConfig: %v", err)
	}
	if !rc.mostSpecificHeaderMutationsWins {
		t.Error("flag should be true")
	}
	if len(rc.requestOps) != 1 || rc.requestOps[0].kind != kindAppend {
		t.Errorf("requestOps: got %+v", rc.requestOps)
	}
	if len(rc.responseOps) != 1 || rc.responseOps[0].kind != kindRemove {
		t.Errorf("responseOps: got %+v", rc.responseOps)
	}
}

func TestRuntimeConfig_QueryParameterMutationsSilentlyIgnored(t *testing.T) {
	// Construct a HeaderMutation with non-empty query_parameter_mutations.
	// The field is deferred per ADR-0112 — should not error; should not produce ops.
	// KeyValueMutation lives in corev3 (envoy/config/core/v3), not mutation_rules/v3.
	// Use Remove field (simplest valid KeyValueMutation construction).
	c := &headermutationv3.HeaderMutation{
		Mutations: &headermutationv3.Mutations{
			QueryParameterMutations: []*corev3.KeyValueMutation{
				{Remove: "q"},
			},
		},
	}
	rc, err := buildRuntimeConfig(c)
	if err != nil {
		t.Fatalf("query_parameter_mutations should be silently ignored; got %v", err)
	}
	if len(rc.requestOps) != 0 || len(rc.responseOps) != 0 {
		t.Errorf("ops should be empty; got requestOps=%d responseOps=%d", len(rc.requestOps), len(rc.responseOps))
	}
}

func TestCompiledMutationOp_AllAppendActionsParse(t *testing.T) {
	cases := []struct {
		action corev3.HeaderValueOption_HeaderAppendAction
		want   corev3.HeaderValueOption_HeaderAppendAction
	}{
		{corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD, corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD},
		{corev3.HeaderValueOption_ADD_IF_ABSENT, corev3.HeaderValueOption_ADD_IF_ABSENT},
		{corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD, corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD},
		{corev3.HeaderValueOption_OVERWRITE_IF_EXISTS, corev3.HeaderValueOption_OVERWRITE_IF_EXISTS},
	}
	for _, tc := range cases {
		t.Run(tc.action.String(), func(t *testing.T) {
			ops, err := compileOps([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x", "v", tc.action)})
			if err != nil || len(ops) != 1 {
				t.Fatalf("compileOps: err=%v ops=%d", err, len(ops))
			}
			if ops[0].appendAction != tc.want {
				t.Errorf("appendAction: got %v, want %v", ops[0].appendAction, tc.want)
			}
		})
	}
}

func TestCompiledMutationOp_RemoveAndAppend(t *testing.T) {
	in := []*commonmutationrulesv3.HeaderMutation{
		mkRemoveOp("x-rm"),
		mkAppendOp("x-add", "v", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
	}
	ops, err := compileOps(in)
	if err != nil || len(ops) != 2 {
		t.Fatalf("compileOps: err=%v ops=%d", err, len(ops))
	}
	if ops[0].kind != kindRemove || ops[0].headerName != "X-Rm" {
		t.Errorf("ops[0]: got %+v", ops[0])
	}
	if ops[1].kind != kindAppend || ops[1].headerName != "X-Add" || ops[1].headerValue != "v" {
		t.Errorf("ops[1]: got %+v", ops[1])
	}
}

func TestRegisterPerRouteValidator(t *testing.T) {
	// The per-route validator is registered via the exported
	// RegisterPerRouteValidator function (called in main.go before
	// httpReg.Freeze). New does NOT call it because New is invoked post-Freeze
	// during listener construction.
	r := envoyhttp.NewHTTPRegistry()
	RegisterPerRouteValidator(r)
	v := r.PerRouteValidator(filterName)
	if v == nil {
		t.Fatal("expected per-route validator registered after RegisterPerRouteValidator; got nil")
	}
	// Sanity: validator accepts a valid HeaderMutationPerRoute.
	okMsg := &headermutationv3.HeaderMutationPerRoute{
		Mutations: &headermutationv3.Mutations{
			RequestMutations: []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x", "v", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)},
		},
	}
	if err := v(okMsg); err != nil {
		t.Errorf("validator should accept valid msg; got %v", err)
	}
	// Validator rejects protected-header mutation.
	badMsg := &headermutationv3.HeaderMutationPerRoute{
		Mutations: &headermutationv3.Mutations{
			RequestMutations: []*commonmutationrulesv3.HeaderMutation{mkAppendOp(":path", "v", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)},
		},
	}
	if err := v(badMsg); err == nil {
		t.Error("validator should reject protected-header mutation; got nil")
	}
}

func TestIsProtectedHeader(t *testing.T) {
	protected := []string{":method", ":path", ":authority", ":scheme", ":status", "host", "Host", "HOST", ":anything"}
	for _, n := range protected {
		if !isProtectedHeader(n) {
			t.Errorf("isProtectedHeader(%q): got false, want true", n)
		}
	}
	allowed := []string{"x-test", "user-agent", "content-length", "x-host-something"}
	for _, n := range allowed {
		if isProtectedHeader(n) {
			t.Errorf("isProtectedHeader(%q): got true, want false", n)
		}
	}
}

func TestApplyOps_AppendIfExistsOrAdd_AbsentTarget(t *testing.T) {
	h := http.Header{}
	applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "v", appendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD}})
	if got := h.Values("X-Test"); len(got) != 1 || got[0] != "v" {
		t.Errorf("got %v, want [v]", got)
	}
}

func TestApplyOps_AppendIfExistsOrAdd_PresentMultiValue(t *testing.T) {
	h := http.Header{}
	h.Add("X-Test", "old1")
	h.Add("X-Test", "old2")
	applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "v", appendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD}})
	if got := h.Values("X-Test"); len(got) != 3 || got[0] != "old1" || got[1] != "old2" || got[2] != "v" {
		t.Errorf("APPEND should preserve prior + add (per §11.4); got %v", got)
	}
}

func TestApplyOps_AddIfAbsent_AbsentTarget(t *testing.T) {
	h := http.Header{}
	applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "v", appendAction: corev3.HeaderValueOption_ADD_IF_ABSENT}})
	if got := h.Get("X-Test"); got != "v" {
		t.Errorf("got %q, want v", got)
	}
}

func TestApplyOps_AddIfAbsent_PresentTarget(t *testing.T) {
	h := http.Header{}
	h.Set("X-Test", "old")
	applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "v", appendAction: corev3.HeaderValueOption_ADD_IF_ABSENT}})
	if got := h.Get("X-Test"); got != "old" {
		t.Errorf("got %q, want old (no-op)", got)
	}
}

func TestApplyOps_OverwriteIfExistsOrAdd_AbsentTarget(t *testing.T) {
	h := http.Header{}
	applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "v", appendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD}})
	if got := h.Get("X-Test"); got != "v" {
		t.Errorf("got %q, want v", got)
	}
}

func TestApplyOps_OverwriteIfExistsOrAdd_PresentMultiValue(t *testing.T) {
	h := http.Header{}
	h.Add("X-Test", "old1")
	h.Add("X-Test", "old2")
	applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "v", appendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD}})
	if got := h.Values("X-Test"); len(got) != 1 || got[0] != "v" {
		t.Errorf("OVERWRITE should collapse multi-value to single (per §11.4); got %v", got)
	}
}

func TestApplyOps_OverwriteIfExists_AbsentTarget(t *testing.T) {
	h := http.Header{}
	applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "v", appendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS}})
	if got := h.Get("X-Test"); got != "" {
		t.Errorf("got %q, want '' (no-op for absent target)", got)
	}
}

func TestApplyOps_OverwriteIfExists_PresentTarget(t *testing.T) {
	h := http.Header{}
	h.Set("X-Test", "old")
	applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "v", appendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS}})
	if got := h.Get("X-Test"); got != "v" {
		t.Errorf("got %q, want v", got)
	}
}

func TestApplyOps_Remove_PresentTarget(t *testing.T) {
	h := http.Header{}
	h.Set("X-Test", "old")
	applyOps(h, []compiledMutationOp{{kind: kindRemove, headerName: "X-Test"}})
	if h.Get("X-Test") != "" {
		t.Errorf("Remove should delete header")
	}
}

func TestApplyOps_Remove_AbsentTarget(t *testing.T) {
	h := http.Header{}
	applyOps(h, []compiledMutationOp{{kind: kindRemove, headerName: "X-Test"}})
	if h.Get("X-Test") != "" {
		t.Errorf("Remove of absent header should be no-op")
	}
}

func TestApplyOps_KeepEmptyValueFalse_EmptyValue_AllAppendActions(t *testing.T) {
	actions := []corev3.HeaderValueOption_HeaderAppendAction{
		corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
		corev3.HeaderValueOption_ADD_IF_ABSENT,
		corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
		corev3.HeaderValueOption_OVERWRITE_IF_EXISTS,
	}
	for _, a := range actions {
		t.Run(a.String(), func(t *testing.T) {
			h := http.Header{}
			// Pre-existing target for OVERWRITE_IF_EXISTS to ensure even with EXISTS-true,
			// the keep_empty_value=false silent-skip fires FIRST per §11.2 conclusion (c).
			h.Set("X-Test", "original")
			applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "", keepEmptyValue: false, appendAction: a}})
			if got := h.Get("X-Test"); got != "original" {
				t.Errorf("%s: keep_empty_value=false + empty value should silent-skip; got %q want original", a, got)
			}
		})
	}
}

func TestApplyOps_KeepEmptyValueTrue_EmptyValue_AppendIfExistsOrAdd(t *testing.T) {
	h := http.Header{}
	applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "", keepEmptyValue: true, appendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD}})
	// Materialize empty value.
	if vs := h.Values("X-Test"); len(vs) != 1 || vs[0] != "" {
		t.Errorf("keep=true + empty + APPEND: got %v, want one empty value", vs)
	}
}

func TestApplyOps_KeepEmptyValueTrue_EmptyValue_OverwriteIfExists_AbsentTarget(t *testing.T) {
	h := http.Header{}
	applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "", keepEmptyValue: true, appendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS}})
	if got := h.Get("X-Test"); got != "" || len(h.Values("X-Test")) != 0 {
		t.Errorf("keep=true + empty + OVERWRITE_IF_EXISTS + absent target: should be no-op (EXISTS gate fires); got %v", h.Values("X-Test"))
	}
}

func TestApplyOps_KeepEmptyValueTrue_EmptyValue_OverwriteIfExists_PresentTarget(t *testing.T) {
	h := http.Header{}
	h.Set("X-Test", "original")
	applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "", keepEmptyValue: true, appendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS}})
	if vs := h.Values("X-Test"); len(vs) != 1 || vs[0] != "" {
		t.Errorf("keep=true + empty + OVERWRITE_IF_EXISTS + present target: should replace with empty; got %v", vs)
	}
}

// fakeDecoderCB is a minimal test impl of DecoderFilterCallbacks supporting
// only RequestRouteConfigsAllTiers + RequestRouteConfig (returns nil) +
// the two no-op callbacks. The other methods panic if called.
type fakeDecoderCB struct {
	route, vhost, rc proto.Message
}

func (f *fakeDecoderCB) ContinueDecoding()                                    {}
func (f *fakeDecoderCB) SendLocalReply(int, string, envoyhttp.OrderedHeaders) {}
func (f *fakeDecoderCB) RequestRouteConfig() proto.Message                    { return nil }
func (f *fakeDecoderCB) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return f.route, f.vhost, f.rc
}
func (f *fakeDecoderCB) EncodeHeaders(http.Header, bool) {}
func (f *fakeDecoderCB) EncodeData([]byte, bool)         {}
func (f *fakeDecoderCB) EncodeTrailers(http.Header)      {}
func (f *fakeDecoderCB) DownstreamPrincipal() []string   { return nil }

// ADR-0165 callback-surface extension stubs (phase-18.2 Task 4).
func (f *fakeDecoderCB) DownstreamRemoteAddr() net.Addr   { return nil }
func (f *fakeDecoderCB) DownstreamLocalAddr() net.Addr    { return nil }
func (f *fakeDecoderCB) DownstreamTLSServerName() string  { return "" }
func (f *fakeDecoderCB) DownstreamTLSPeerCertDER() []byte { return nil }
func (f *fakeDecoderCB) DownstreamProtocol() string       { return "" }
func (f *fakeDecoderCB) ListenerPrincipal() string        { return "" }

// ADR-0192 callback-surface extension stubs (phase-22.2 Task 5).
func (f *fakeDecoderCB) DownstreamTLSConnectionState() *tls.ConnectionState { return nil }
func (f *fakeDecoderCB) DynamicMetadata() *dynamicmetadata.Bucket           { return nil }

// ADR-0198 callback-surface extension stubs (phase-24.1 Task 5 — DELTA-2).
func (f *fakeDecoderCB) RouteRateLimits() []*routev3.RateLimit       { return nil }
func (f *fakeDecoderCB) VirtualHostRateLimits() []*routev3.RateLimit { return nil }

func mkPerRoute(req, resp []*commonmutationrulesv3.HeaderMutation) *headermutationv3.HeaderMutationPerRoute {
	return &headermutationv3.HeaderMutationPerRoute{
		Mutations: &headermutationv3.Mutations{RequestMutations: req, ResponseMutations: resp},
	}
}

func mkFilterFromMutation(t *testing.T, mut *headermutationv3.HeaderMutation, dcb envoyhttp.DecoderFilterCallbacks) *filter {
	t.Helper()
	factory, err := New(mustAny(t, mut), envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inst := factory()
	f := inst.Decoder.(*filter)
	f.SetDecoderCallbacks(dcb)
	return f
}

func TestDecodeHeaders_ListenerLevel_NoPerRoute(t *testing.T) {
	mut := &headermutationv3.HeaderMutation{
		Mutations: &headermutationv3.Mutations{
			RequestMutations: []*commonmutationrulesv3.HeaderMutation{
				mkAppendOp("x-test", "listener", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
				mkRemoveOp("user-agent"),
			},
		},
	}
	dcb := &fakeDecoderCB{} // no per-route configs
	f := mkFilterFromMutation(t, mut, dcb)
	h := http.Header{}
	h.Set("user-agent", "curl/8.20")
	if status := f.DecodeHeaders(h, false); status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue", status)
	}
	if h.Get("X-Test") != "listener" {
		t.Errorf("x-test: got %q, want listener", h.Get("X-Test"))
	}
	if h.Get("User-Agent") != "" {
		t.Errorf("user-agent should be removed; got %q", h.Get("User-Agent"))
	}
}

func TestDecodeHeaders_PerRoute_RouteOnly(t *testing.T) {
	mut := &headermutationv3.HeaderMutation{
		Mutations: &headermutationv3.Mutations{
			RequestMutations: []*commonmutationrulesv3.HeaderMutation{
				mkAppendOp("x-test", "listener", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
			},
		},
	}
	routePR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{
		mkAppendOp("x-test", "route", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
	}, nil)
	dcb := &fakeDecoderCB{route: routePR}
	f := mkFilterFromMutation(t, mut, dcb)
	h := http.Header{}
	f.DecodeHeaders(h, false)
	if got := h.Get("X-Test"); got != "route" {
		t.Errorf("got %q, want route (route applied after listener)", got)
	}
}

func TestDecodeHeaders_MultiTier_FlagFalse(t *testing.T) {
	mut := &headermutationv3.HeaderMutation{
		Mutations: &headermutationv3.Mutations{
			RequestMutations: []*commonmutationrulesv3.HeaderMutation{
				mkAppendOp("x-test", "listener", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
			},
		},
		MostSpecificHeaderMutationsWins: false,
	}
	routePR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "route", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
	vhPR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "vh", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
	rcPR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "rc", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
	dcb := &fakeDecoderCB{route: routePR, vhost: vhPR, rc: rcPR}
	f := mkFilterFromMutation(t, mut, dcb)
	h := http.Header{}
	f.DecodeHeaders(h, false)
	// flag=false: Route → VHost → RC (RC applied LAST, wins overlap) per §11.5
	if got := h.Get("X-Test"); got != "rc" {
		t.Errorf("flag=false: got %q, want rc (least-specific wins per §11.5)", got)
	}
}

func TestDecodeHeaders_MultiTier_FlagTrue(t *testing.T) {
	mut := &headermutationv3.HeaderMutation{
		Mutations: &headermutationv3.Mutations{
			RequestMutations: []*commonmutationrulesv3.HeaderMutation{
				mkAppendOp("x-test", "listener", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
			},
		},
		MostSpecificHeaderMutationsWins: true,
	}
	routePR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "route", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
	vhPR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "vh", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
	rcPR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "rc", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
	dcb := &fakeDecoderCB{route: routePR, vhost: vhPR, rc: rcPR}
	f := mkFilterFromMutation(t, mut, dcb)
	h := http.Header{}
	f.DecodeHeaders(h, false)
	// flag=true: RC → VHost → Route (Route applied LAST, wins overlap) per §11.5
	if got := h.Get("X-Test"); got != "route" {
		t.Errorf("flag=true: got %q, want route (most-specific wins per §11.5)", got)
	}
}

func TestDecodeHeaders_MultiTier_TwoOfThree_RouteAndVHost(t *testing.T) {
	mut := &headermutationv3.HeaderMutation{Mutations: &headermutationv3.Mutations{}, MostSpecificHeaderMutationsWins: false}
	routePR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "route", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
	vhPR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "vh", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
	dcb := &fakeDecoderCB{route: routePR, vhost: vhPR} // rc nil
	f := mkFilterFromMutation(t, mut, dcb)
	h := http.Header{}
	f.DecodeHeaders(h, false)
	// flag=false: Route → VHost (RC nil — skipped); VHost wins
	if got := h.Get("X-Test"); got != "vh" {
		t.Errorf("got %q, want vh (route + vhost; VHost applied last)", got)
	}
}

func TestDecodeHeaders_MultiTier_TwoOfThree_RouteAndRC(t *testing.T) {
	mut := &headermutationv3.HeaderMutation{Mutations: &headermutationv3.Mutations{}, MostSpecificHeaderMutationsWins: false}
	routePR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "route", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
	rcPR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "rc", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
	dcb := &fakeDecoderCB{route: routePR, rc: rcPR} // vhost nil
	f := mkFilterFromMutation(t, mut, dcb)
	h := http.Header{}
	f.DecodeHeaders(h, false)
	if got := h.Get("X-Test"); got != "rc" {
		t.Errorf("got %q, want rc (route + rc; RC applied last)", got)
	}
}

func TestDecodeHeaders_MultiTier_TwoOfThree_VHostAndRC(t *testing.T) {
	mut := &headermutationv3.HeaderMutation{Mutations: &headermutationv3.Mutations{}, MostSpecificHeaderMutationsWins: false}
	vhPR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "vh", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
	rcPR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "rc", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
	dcb := &fakeDecoderCB{vhost: vhPR, rc: rcPR}
	f := mkFilterFromMutation(t, mut, dcb)
	h := http.Header{}
	f.DecodeHeaders(h, false)
	if got := h.Get("X-Test"); got != "rc" {
		t.Errorf("got %q, want rc (vh + rc; RC applied last)", got)
	}
}

func TestDecodeHeaders_NilDecoderCallbacks_AppliesListenerOnly(t *testing.T) {
	mut := &headermutationv3.HeaderMutation{
		Mutations: &headermutationv3.Mutations{
			RequestMutations: []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "listener", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)},
		},
	}
	factory, _ := New(mustAny(t, mut), envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
	inst := factory()
	f := inst.Decoder.(*filter)
	// Do NOT SetDecoderCallbacks — exercise the dcb-nil branch.
	h := http.Header{}
	if status := f.DecodeHeaders(h, false); status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue", status)
	}
	if got := h.Get("X-Test"); got != "listener" {
		t.Errorf("listener-only path: got %q, want listener", got)
	}
}

func TestEncodeHeaders_Symmetric(t *testing.T) {
	mut := &headermutationv3.HeaderMutation{
		Mutations: &headermutationv3.Mutations{
			ResponseMutations: []*commonmutationrulesv3.HeaderMutation{
				mkAppendOp("x-resp", "listener-resp", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
				mkAppendOp("x-multi", "APPENDED", corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD),
			},
		},
	}
	dcb := &fakeDecoderCB{}
	f := mkFilterFromMutation(t, mut, dcb)
	h := http.Header{}
	h.Add("X-Multi", "alpha")
	h.Add("X-Multi", "beta")
	if status := f.EncodeHeaders(h, false); status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue", status)
	}
	if got := h.Get("X-Resp"); got != "listener-resp" {
		t.Errorf("x-resp: got %q, want listener-resp", got)
	}
	if got := h.Values("X-Multi"); len(got) != 3 || got[0] != "alpha" || got[1] != "beta" || got[2] != "APPENDED" {
		t.Errorf("x-multi APPEND should preserve + add (per §11.4); got %v", got)
	}
}

func TestEncodeHeaders_MultiTier_FlagFalse_ResponseSide(t *testing.T) {
	mut := &headermutationv3.HeaderMutation{
		Mutations: &headermutationv3.Mutations{
			ResponseMutations: []*commonmutationrulesv3.HeaderMutation{
				mkAppendOp("x-resp", "listener-resp", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
			},
		},
		MostSpecificHeaderMutationsWins: false,
	}
	routePR := mkPerRoute(nil, []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-resp", "route-resp", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)})
	vhPR := mkPerRoute(nil, []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-resp", "vh-resp", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)})
	rcPR := mkPerRoute(nil, []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-resp", "rc-resp", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)})
	dcb := &fakeDecoderCB{route: routePR, vhost: vhPR, rc: rcPR}
	f := mkFilterFromMutation(t, mut, dcb)
	h := http.Header{}
	f.EncodeHeaders(h, false)
	if got := h.Get("X-Resp"); got != "rc-resp" {
		t.Errorf("flag=false response: got %q, want rc-resp", got)
	}
}

func TestEncodeHeaders_MultiTier_FlagTrue_ResponseSide(t *testing.T) {
	mut := &headermutationv3.HeaderMutation{
		Mutations:                       &headermutationv3.Mutations{},
		MostSpecificHeaderMutationsWins: true,
	}
	routePR := mkPerRoute(nil, []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-resp", "route-resp", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)})
	vhPR := mkPerRoute(nil, []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-resp", "vh-resp", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)})
	rcPR := mkPerRoute(nil, []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-resp", "rc-resp", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)})
	dcb := &fakeDecoderCB{route: routePR, vhost: vhPR, rc: rcPR}
	f := mkFilterFromMutation(t, mut, dcb)
	h := http.Header{}
	f.EncodeHeaders(h, false)
	if got := h.Get("X-Resp"); got != "route-resp" {
		t.Errorf("flag=true response: got %q, want route-resp", got)
	}
}

func TestHeaderMutation_MultiTierConcurrentRequests(t *testing.T) {
	// Race-detector cycle test per SPEC §12 deferred decision 7. Spawn many
	// goroutines that each construct a fresh *filter from the SAME factory
	// (sharing the closure-captured *runtimeConfig) and call DecodeHeaders +
	// EncodeHeaders concurrently. The framework's per-instance discipline +
	// *runtimeConfig read-only-after-New invariant make this safe by
	// construction; the race detector validates.
	mut := &headermutationv3.HeaderMutation{
		Mutations: &headermutationv3.Mutations{
			RequestMutations:  []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-req", "v", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)},
			ResponseMutations: []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-resp", "v", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)},
		},
	}
	factory, err := New(mustAny(t, mut), envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	routePR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-tier", "route", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inst := factory()
			f := inst.Decoder.(*filter)
			f.SetDecoderCallbacks(&fakeDecoderCB{route: routePR})
			h := http.Header{}
			f.DecodeHeaders(h, false)
			f.EncodeHeaders(h, false)
		}()
	}
	wg.Wait()
}
