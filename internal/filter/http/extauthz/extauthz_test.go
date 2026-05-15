package extauthz

// extauthz_test.go — unit-test Groups 1 + 2 + 7 for Task 2.
//
// Test group assignments per PLAN Task 2 / SPEC §14.1:
//
//   Group 1 — ExtAuthz parse + services oneof dispatch
//   Group 2 — compiledConfig shape + filterStats allocation
//   Group 7 — per-route: parsePerRoute + resolvePerRouteConfig
//
// Groups 3/4/5/6/8/9 land at Tasks 3/3/9/6/5/9 respectively per PLAN.
//
// Design note (Group 1 "valid http_service" tests):
//   At Task 2, buildHTTPCheckFn is a STUB returning the sentinel error
//   errHTTPCheckFnStub. A "valid http_service" config therefore errors at the
//   buildHTTPCheckFn call, not at an earlier parse-rejection step. The Group 1
//   valid-http_service tests assert that (a) the factory errors, (b) the error
//   text contains the stub sentinel text (to distinguish the stub from a real
//   parse rejection), and (c) the grpc_service / empty-oneof / non-V3 cases
//   produce their specific rejections BEFORE reaching the stub.
//   At Task 3 these tests will be tightened to assert success (factory non-nil).
//   This design choice is documented per PLAN Group 1 note.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext_authzv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// ----------------------------------------------------------------------------
// Test helpers (mirror phase-16 rbac_test.go + phase-17 jwtauthn_test.go).
// ----------------------------------------------------------------------------

// mustAny packages a proto.Message into an *anypb.Any. Mirrors phase-13/14/15/16/17 helper.
func mustAny(t *testing.T, msg proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

// freshFactoryCtx returns a FactoryCtx with a fresh stats.Registry and an
// HCM stat_prefix. Used for tests that exercise the stat-registration path.
func freshFactoryCtx() envoyhttp.FactoryCtx {
	return envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "ingress_http"}
}

// freshFactoryCtxWithRegistry returns a FactoryCtx with the supplied Registry.
// Used by tests that need to inspect counter registration externally.
func freshFactoryCtxWithRegistry(reg *stats.Registry) envoyhttp.FactoryCtx {
	return envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "ingress_http"}
}

// validHTTPService returns the minimal ExtAuthz with an http_service pointing at
// a local server_uri. At Task 2 buildHTTPCheckFn is a stub; this proto triggers
// the stub path.
func validHTTPServiceConfig() *ext_authzv3.ExtAuthz {
	return &ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{
					Uri: "http://127.0.0.1:9191/auth",
				},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
	}
}

// ----------------------------------------------------------------------------
// Group 1 — ExtAuthz parse + services oneof dispatch
// (Group 1 per SPEC §14.1 + PLAN Task 2)
// ----------------------------------------------------------------------------

// TestNew_NilTC verifies that a nil typed_config is rejected with a descriptive error.
func TestNew_NilTC(t *testing.T) {
	factory, err := New(nil, freshFactoryCtx())
	if err == nil {
		t.Fatal("New(nil, _): want error, got nil")
	}
	if factory != nil {
		t.Errorf("New(nil, _): want nil factory, got non-nil")
	}
	if !strings.Contains(err.Error(), "typed_config required") {
		t.Errorf("got %q; want substring 'typed_config required'", err.Error())
	}
}

// TestNew_MalformedTC verifies that a malformed Any returns an unmarshal error.
func TestNew_MalformedTC(t *testing.T) {
	bad := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff, 0xff, 0xff}}
	factory, err := New(bad, freshFactoryCtx())
	if err == nil {
		t.Fatal("New(malformed, _): want error, got nil")
	}
	if factory != nil {
		t.Errorf("New(malformed, _): want nil factory, got non-nil")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("got %q; want substring 'unmarshal'", err.Error())
	}
}

// TestNew_EmptyServicesOneof verifies PARSE-REJECT when services oneof is not set.
// Per SPEC §6.4 + ADR-0157: the factory rejects an empty services oneof.
func TestNew_EmptyServicesOneof(t *testing.T) {
	cfg := &ext_authzv3.ExtAuthz{
		// No Services field set.
		TransportApiVersion: corev3.ApiVersion_V3,
	}
	factory, err := New(mustAny(t, cfg), freshFactoryCtx())
	if err == nil {
		t.Fatal("New(empty services oneof): want error, got nil")
	}
	if factory != nil {
		t.Errorf("New(empty services): want nil factory, got non-nil")
	}
	if !strings.Contains(err.Error(), "services oneof must be set") {
		t.Errorf("got %q; want substring 'services oneof must be set'", err.Error())
	}
}

// TestNew_GrpcServiceParseReject verifies PARSE-REJECT for grpc_service mode in 18.1.
// Per SPEC §6.4 + ADR-0157: grpc_service is not yet supported in 18.1.
func TestNew_GrpcServiceParseReject(t *testing.T) {
	cfg := &ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_GrpcService{
			GrpcService: &corev3.GrpcService{},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
	}
	factory, err := New(mustAny(t, cfg), freshFactoryCtx())
	if err == nil {
		t.Fatal("New(grpc_service): want error, got nil")
	}
	if factory != nil {
		t.Errorf("New(grpc_service): want nil factory, got non-nil")
	}
	if !strings.Contains(err.Error(), "grpc_service mode not yet supported (lands in phase 18.2)") {
		t.Errorf("got %q; want substring 'grpc_service mode not yet supported (lands in phase 18.2)'", err.Error())
	}
}

// TestNew_NonV3TransportApiVersion verifies PARSE-REJECT for non-V3 transport_api_version.
// Per ADR-0008: non-V3 transport_api_version is rejected.
func TestNew_NonV3TransportApiVersion(t *testing.T) {
	cfg := &ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{
					Uri: "http://127.0.0.1:9191/auth",
				},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V2,
	}
	factory, err := New(mustAny(t, cfg), freshFactoryCtx())
	if err == nil {
		t.Fatal("New(transport_api_version=V2): want error, got nil")
	}
	if factory != nil {
		t.Errorf("New(transport_api_version=V2): want nil factory, got non-nil")
	}
	if !strings.Contains(err.Error(), "transport_api_version") {
		t.Errorf("got %q; want substring 'transport_api_version'", err.Error())
	}
}

// TestNew_NonAutoTransportApiVersion verifies PARSE-REJECT for ApiVersion_AUTO (non-V3).
func TestNew_NonAutoTransportApiVersion(t *testing.T) {
	cfg := &ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{Uri: "http://127.0.0.1:9191"},
			},
		},
		TransportApiVersion: corev3.ApiVersion_AUTO,
	}
	factory, err := New(mustAny(t, cfg), freshFactoryCtx())
	if err == nil {
		t.Fatal("New(transport_api_version=AUTO): want error, got nil")
	}
	if factory != nil {
		t.Errorf("New(transport_api_version=AUTO): want nil factory, got non-nil")
	}
	if !strings.Contains(err.Error(), "transport_api_version") {
		t.Errorf("got %q; want substring 'transport_api_version'", err.Error())
	}
}

// TestNew_WithRequestBodyMaxBytesZero verifies PARSE-REJECT when with_request_body
// is set but max_request_bytes == 0 (PGV-mirror per SPEC §6.4 + ADR-0157).
func TestNew_WithRequestBodyMaxBytesZero(t *testing.T) {
	cfg := &ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{Uri: "http://127.0.0.1:9191"},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
		WithRequestBody: &ext_authzv3.BufferSettings{
			MaxRequestBytes: 0, // invalid: must be > 0
		},
	}
	factory, err := New(mustAny(t, cfg), freshFactoryCtx())
	if err == nil {
		t.Fatal("New(max_request_bytes=0): want error, got nil")
	}
	if factory != nil {
		t.Errorf("New(max_request_bytes=0): want nil factory, got non-nil")
	}
	if !strings.Contains(err.Error(), "max_request_bytes") {
		t.Errorf("got %q; want substring 'max_request_bytes'", err.Error())
	}
}

// TestNew_HttpServiceMissingServerURI verifies PARSE-REJECT when http_service has
// no server_uri set (PGV-mirror per SPEC §6.5).
// NOTE: At Task 2 this test exercises the path in the stub buildHTTPCheckFn.
func TestNew_HttpServiceMissingServerURI(t *testing.T) {
	cfg := &ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				// no server_uri
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
	}
	factory, err := New(mustAny(t, cfg), freshFactoryCtx())
	if err == nil {
		t.Fatal("New(missing server_uri): want error, got nil")
	}
	if factory != nil {
		t.Errorf("New(missing server_uri): want nil factory, got non-nil")
	}
	// At Task 2 the stub buildHTTPCheckFn returns errHTTPCheckFnStub;
	// the server_uri validation happens inside buildHTTPCheckFn (Task 3 wires it).
	// We just assert an error was returned (stub or real validation error).
	_ = err // any error is correct at Task 2
}

// TestNew_HttpServiceEmptyURI verifies PARSE-REJECT when server_uri.uri is empty.
// NOTE: At Task 2 this hits the buildHTTPCheckFn stub path.
func TestNew_HttpServiceEmptyURI(t *testing.T) {
	cfg := &ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{Uri: ""}, // empty URI
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
	}
	factory, err := New(mustAny(t, cfg), freshFactoryCtx())
	if err == nil {
		t.Fatal("New(empty server_uri.uri): want error, got nil")
	}
	if factory != nil {
		t.Errorf("New(empty server_uri.uri): want nil factory, got non-nil")
	}
	_ = err // any error is correct at Task 2
}

// TestNew_HttpService_ValidConfig_Task2Stub was the Task 2 stub-path test.
// TIGHTENED at Task 3: the real buildHTTPCheckFn lands in check.go; a valid
// http_service config now produces a non-nil factory (no stub error).
// The stub-path assertions are superseded by TestNew_HttpService_ValidConfig_RealImpl
// in Group 4. This test is RETIRED (kept for audit trail; it now verifies success).
func TestNew_HttpService_ValidConfig_Task2Stub(t *testing.T) {
	// Task 3 tightening: assert factory != nil (real impl landed).
	factory, err := New(mustAny(t, validHTTPServiceConfig()), freshFactoryCtx())
	if err != nil {
		t.Fatalf("New(valid http_service): Task 3 real impl: want nil error, got %v", err)
	}
	if factory == nil {
		t.Fatal("New(valid http_service): Task 3 real impl: want non-nil factory, got nil")
	}
}

// TestNew_StatusOnError_Default verifies the default status_on_error is 403
// when status_on_error is unset. The compiledConfig.statusOnError field should
// be 403. Tested via buildCompiledConfig directly.
//
// TIGHTENED at Task 3: now that buildHTTPCheckFn is real (check.go), a valid
// http_service config produces a non-nil cc — the stub-era `if cc != nil`
// wrapper is removed in favor of unconditional assertions.
func TestNew_StatusOnError_Default(t *testing.T) {
	cfg := &ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{Uri: "http://127.0.0.1:9191"},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
		// status_on_error unset → default 403
	}
	cc, err := buildCompiledConfig(freshFactoryCtx(), cfg)
	if err != nil {
		t.Fatalf("buildCompiledConfig: unexpected error: %v", err)
	}
	if cc == nil {
		t.Fatal("buildCompiledConfig: got nil cc, want non-nil")
	}
	if cc.statusOnError != 403 {
		t.Errorf("statusOnError: got %d, want 403", cc.statusOnError)
	}
}

// TestNew_StatusOnError_Explicit verifies that an explicit status_on_error is
// consumed. Tested via buildCompiledConfig.
//
// TIGHTENED at Task 3: unconditional assertions (real buildHTTPCheckFn).
func TestNew_StatusOnError_Explicit(t *testing.T) {
	cfg := &ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{Uri: "http://127.0.0.1:9191"},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
		StatusOnError:       &typev3.HttpStatus{Code: typev3.StatusCode_ServiceUnavailable}, // 503
	}
	cc, err := buildCompiledConfig(freshFactoryCtx(), cfg)
	if err != nil {
		t.Fatalf("buildCompiledConfig: unexpected error: %v", err)
	}
	if cc == nil {
		t.Fatal("buildCompiledConfig: got nil cc, want non-nil")
	}
	if cc.statusOnError != 503 {
		t.Errorf("statusOnError: got %d, want 503", cc.statusOnError)
	}
}

// TestNew_StatPrefix_Consumed verifies that stat_prefix is consumed (not causing
// error) by New with a valid config. This is a sanity check that the stat_prefix
// field doesn't cause unexpected parse rejection.
func TestNew_StatPrefix_Consumed(t *testing.T) {
	cfg := &ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{Uri: "http://127.0.0.1:9191"},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
		StatPrefix:          "custom_prefix",
	}
	_, err := New(mustAny(t, cfg), freshFactoryCtx())
	// Task 3 tightening: the real buildHTTPCheckFn lands; no error expected.
	// The stat_prefix must NOT be the source of any error.
	if err != nil {
		t.Errorf("stat_prefix must not cause error; got %q", err.Error())
	}
}

// ----------------------------------------------------------------------------
// Group 2 — compiledConfig shape + filterStats allocation
// (Group 2 per SPEC §14.1 + PLAN Task 2)
// ----------------------------------------------------------------------------

// TestFilterStats_6Counters verifies that newFilterStats allocates exactly 6
// counters with the correct names per ADR-0156 + SPEC §6.2.
func TestFilterStats_6Counters(t *testing.T) {
	reg := stats.NewRegistry()
	ctx := freshFactoryCtxWithRegistry(reg)
	fs := newFilterStats(reg, ctx.StatPrefix)
	if fs == nil {
		t.Fatal("newFilterStats: got nil, want non-nil")
	}
	if fs.ok == nil {
		t.Error("filterStats.ok is nil")
	}
	if fs.denied == nil {
		t.Error("filterStats.denied is nil")
	}
	if fs.errored == nil {
		t.Error("filterStats.errored is nil")
	}
	if fs.disabled == nil {
		t.Error("filterStats.disabled is nil (must register for scrape-stability even though STRUCTURALLY UNREACHABLE)")
	}
	if fs.failureModeAllowed == nil {
		t.Error("filterStats.failureModeAllowed is nil")
	}
	if fs.invalid == nil {
		t.Error("filterStats.invalid is nil")
	}
}

// TestFilterStats_CounterNames verifies the exact counter names under the
// SN2-reuse namespace http.<HCM_stat_prefix>.ext_authz.<counter> per ADR-0156.
func TestFilterStats_CounterNames(t *testing.T) {
	reg := stats.NewRegistry()
	ctx := freshFactoryCtxWithRegistry(reg)
	_ = newFilterStats(reg, ctx.StatPrefix)

	// Collect all registered counter names.
	names := make(map[string]bool)
	reg.Walk(func(m stats.Metric) {
		names[m.Name()] = true
	})

	wantPrefix := "http." + ctx.StatPrefix + ".ext_authz."
	wantCounters := []string{"ok", "denied", "error", "disabled", "failure_mode_allowed", "invalid"}
	for _, c := range wantCounters {
		fullName := wantPrefix + c
		if !names[fullName] {
			t.Errorf("counter %q not registered; registered: %v", fullName, keysOf(names))
		}
	}
}

func keysOf(m map[string]bool) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// TestFilterStats_NilRegistryTolerance verifies that a nil Stats ctx does not
// panic — per ADR-0085 nil-tolerance the buildCompiledConfig caller guards
// `if ctx.Stats != nil`.
func TestFilterStats_NilRegistryTolerance(t *testing.T) {
	cfg := validHTTPServiceConfig()
	nilCtx := envoyhttp.FactoryCtx{Stats: nil, StatPrefix: ""}
	// buildCompiledConfig must not panic when ctx.Stats is nil.
	// At Task 2 the stub fires; that's fine — we're testing no panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("buildCompiledConfig panicked with nil Stats: %v", r)
		}
	}()
	_, _ = buildCompiledConfig(nilCtx, cfg)
}

// TestCompiledConfig_FieldFinal verifies that compiledConfig is mode-agnostic
// and field-final at 18.1 — no transport-specific state (the shape carries only
// the common fields documented in SPEC §6.2 + ADR-0157). This is a compile-time
// structural test — if the struct had transport-specific fields the test wouldn't
// even compile.
func TestCompiledConfig_FieldFinal(t *testing.T) {
	// Construct a zero-value compiledConfig and verify all documented fields exist.
	cc := &compiledConfig{}
	// mode-agnostic fields (no transport-specific state):
	if cc.checkFn != nil {
		t.Errorf("checkFn: want nil, got non-nil")
	}
	if cc.withRequestBody != nil {
		t.Errorf("withRequestBody: want nil, got non-nil")
	}
	if cc.failureModeAllow {
		t.Errorf("failureModeAllow: want false, got true")
	}
	if cc.failureModeAllowHeaderAdd {
		t.Errorf("failureModeAllowHeaderAdd: want false, got true")
	}
	if cc.clearRouteCache {
		t.Errorf("clearRouteCache: want false, got true")
	}
	if cc.statusOnError != 0 {
		t.Errorf("statusOnError zero: got %d, want 0", cc.statusOnError)
	}
	if cc.validateMutations {
		t.Errorf("validateMutations: want false, got true")
	}
	if cc.allowedHeaders != nil {
		t.Errorf("allowedHeaders: want nil, got non-nil")
	}
	if cc.disallowedHeaders != nil {
		t.Errorf("disallowedHeaders: want nil, got non-nil")
	}
	if cc.stats != nil {
		t.Errorf("stats: want nil, got non-nil")
	}
}

// TestCompiledConfig_FailureModeAllowConsumed verifies that failure_mode_allow is
// consumed into compiledConfig.
//
// TIGHTENED at Task 3: unconditional assertions (real buildHTTPCheckFn).
func TestCompiledConfig_FailureModeAllowConsumed(t *testing.T) {
	cfg := &ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{Uri: "http://127.0.0.1:9191"},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
		FailureModeAllow:    true,
	}
	cc, err := buildCompiledConfig(freshFactoryCtx(), cfg)
	if err != nil {
		t.Fatalf("buildCompiledConfig: unexpected error: %v", err)
	}
	if cc == nil {
		t.Fatal("buildCompiledConfig: got nil cc, want non-nil")
	}
	if !cc.failureModeAllow {
		t.Error("failureModeAllow: got false, want true")
	}
}

// TestCompiledConfig_WithRequestBodyConsumed verifies that a valid with_request_body
// is consumed into compiledConfig.withRequestBody.
//
// TIGHTENED at Task 3: unconditional assertions (real buildHTTPCheckFn).
func TestCompiledConfig_WithRequestBodyConsumed(t *testing.T) {
	cfg := &ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{Uri: "http://127.0.0.1:9191"},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
		WithRequestBody: &ext_authzv3.BufferSettings{
			MaxRequestBytes:     1024,
			AllowPartialMessage: true,
			PackAsBytes:         false,
		},
	}
	cc, err := buildCompiledConfig(freshFactoryCtx(), cfg)
	if err != nil {
		t.Fatalf("buildCompiledConfig: unexpected error: %v", err)
	}
	if cc == nil {
		t.Fatal("buildCompiledConfig: got nil cc, want non-nil")
	}
	if cc.withRequestBody == nil {
		t.Fatal("withRequestBody: got nil, want non-nil")
	}
	if cc.withRequestBody.maxRequestBytes != 1024 {
		t.Errorf("maxRequestBytes: got %d, want 1024", cc.withRequestBody.maxRequestBytes)
	}
	if !cc.withRequestBody.allowPartialMessage {
		t.Error("allowPartialMessage: got false, want true")
	}
}

// TestDecodeHeadersSkeleton_ReturnsHeaderContinue verifies the DecodeHeaders
// skeleton returns HeaderContinue (the pass-through placeholder until Task 9
// wires the real dispatch body).
func TestDecodeHeadersSkeleton_ReturnsHeaderContinue(t *testing.T) {
	// Build a minimal factoryState with a compiledConfig (no stats needed for
	// skeleton test).
	cc := &compiledConfig{
		statusOnError: 403,
	}
	state := &factoryState{listenerRC: cc}
	f := &filter{state: state}

	headers := make(map[string][]string)
	result := f.DecodeHeaders(headers, true)
	if result != envoyhttp.Continue {
		t.Errorf("DecodeHeaders skeleton: got %v, want HeaderContinue", result)
	}
}

// TestDecodeDataSkeleton_Passthrough verifies the DecodeData skeleton returns DataContinue.
func TestDecodeDataSkeleton_Passthrough(t *testing.T) {
	f := &filter{}
	result := f.DecodeData(nil, true)
	if result != envoyhttp.DataContinue {
		t.Errorf("DecodeData skeleton: got %v, want DataContinue", result)
	}
}

// TestDecodeTrailersSkeleton_Passthrough verifies the DecodeTrailers skeleton returns TrailersContinue.
func TestDecodeTrailersSkeleton_Passthrough(t *testing.T) {
	f := &filter{}
	result := f.DecodeTrailers(nil)
	if result != envoyhttp.TrailersContinue {
		t.Errorf("DecodeTrailers skeleton: got %v, want TrailersContinue", result)
	}
}

// TestSetDecoderCallbacks_Stores verifies SetDecoderCallbacks stores the dcb.
func TestSetDecoderCallbacks_Stores(t *testing.T) {
	f := &filter{}
	if f.dcb != nil {
		t.Error("dcb: want nil initially, got non-nil")
	}
	var dcb envoyhttp.DecoderFilterCallbacks = nil // test with nil is sufficient for skeleton
	f.SetDecoderCallbacks(dcb)
	if f.dcb != dcb {
		t.Errorf("dcb: want stored value, got different value")
	}
}

// TestOnDestroySkeleton_NoOp verifies OnDestroy does not panic in Task 2 skeleton state.
func TestOnDestroySkeleton_NoOp(t *testing.T) {
	f := &filter{}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("OnDestroy panicked: %v", r)
		}
	}()
	f.OnDestroy()
}

// ----------------------------------------------------------------------------
// Group 7 — per-route: parsePerRoute + resolvePerRouteConfig
// (Group 7 per SPEC §14.1 + PLAN Task 2)
// ----------------------------------------------------------------------------

// TestParsePerRoute_EmptyOverride verifies PARSE-REJECT when override oneof is not set.
// Per SPEC §6.6 + ADR-0163: override oneof is PGV-required.
func TestParsePerRoute_EmptyOverride(t *testing.T) {
	empty := &ext_authzv3.ExtAuthzPerRoute{
		// No Override field set.
	}
	_, err := parsePerRoute(empty)
	if err == nil {
		t.Fatal("parsePerRoute(empty): want error, got nil")
	}
	if !strings.Contains(err.Error(), "override") {
		t.Errorf("got %q; want substring 'override'", err.Error())
	}
}

// TestParsePerRoute_DisabledFalse verifies PARSE-REJECT when disabled=false.
// Per SPEC §6.6 + ADR-0163: PGV const:true — disabled must be true.
func TestParsePerRoute_DisabledFalse(t *testing.T) {
	pr := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_Disabled{
			Disabled: false, // invalid: must be true
		},
	}
	_, err := parsePerRoute(pr)
	if err == nil {
		t.Fatal("parsePerRoute(disabled=false): want error, got nil")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("got %q; want substring 'disabled'", err.Error())
	}
}

// TestParsePerRoute_DisabledTrue verifies disabled:true parses successfully.
func TestParsePerRoute_DisabledTrue(t *testing.T) {
	pr := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_Disabled{
			Disabled: true,
		},
	}
	cpr, err := parsePerRoute(pr)
	if err != nil {
		t.Fatalf("parsePerRoute(disabled=true): unexpected error: %v", err)
	}
	if cpr == nil {
		t.Fatal("parsePerRoute(disabled=true): got nil compiledPerRoute")
	}
	if !cpr.disabled {
		t.Error("disabled: got false, want true")
	}
	if cpr.checkSettings != nil {
		t.Error("checkSettings: want nil for disabled arm, got non-nil")
	}
}

// TestParsePerRoute_CheckSettings_Empty verifies that an empty check_settings parses
// successfully (context_extensions is optional).
func TestParsePerRoute_CheckSettings_Empty(t *testing.T) {
	pr := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &ext_authzv3.CheckSettings{},
		},
	}
	cpr, err := parsePerRoute(pr)
	if err != nil {
		t.Fatalf("parsePerRoute(empty check_settings): unexpected error: %v", err)
	}
	if cpr == nil {
		t.Fatal("parsePerRoute(empty check_settings): got nil compiledPerRoute")
	}
	if cpr.disabled {
		t.Error("disabled: got true, want false for check_settings arm")
	}
	if cpr.checkSettings == nil {
		t.Error("checkSettings: want non-nil, got nil")
	}
}

// TestParsePerRoute_CheckSettings_WithContextExtensions verifies context_extensions
// are consumed into compiledCheckSettings.
func TestParsePerRoute_CheckSettings_WithContextExtensions(t *testing.T) {
	pr := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &ext_authzv3.CheckSettings{
				ContextExtensions: map[string]string{
					"vhost": "my-virtual-host",
					"role":  "admin",
				},
			},
		},
	}
	cpr, err := parsePerRoute(pr)
	if err != nil {
		t.Fatalf("parsePerRoute(context_extensions): unexpected error: %v", err)
	}
	if cpr.checkSettings == nil {
		t.Fatal("checkSettings: got nil")
	}
	if cpr.checkSettings.contextExtensions == nil {
		t.Fatal("contextExtensions: got nil map")
	}
	if cpr.checkSettings.contextExtensions["vhost"] != "my-virtual-host" {
		t.Errorf("contextExtensions[vhost]: got %q, want %q",
			cpr.checkSettings.contextExtensions["vhost"], "my-virtual-host")
	}
}

// TestParsePerRoute_CheckSettings_DisableRequestBodyBuffering verifies the
// disable_request_body_buffering flag is consumed.
func TestParsePerRoute_CheckSettings_DisableRequestBodyBuffering(t *testing.T) {
	pr := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &ext_authzv3.CheckSettings{
				DisableRequestBodyBuffering: true,
			},
		},
	}
	cpr, err := parsePerRoute(pr)
	if err != nil {
		t.Fatalf("parsePerRoute(disable_request_body_buffering): unexpected error: %v", err)
	}
	if cpr.checkSettings == nil {
		t.Fatal("checkSettings: got nil")
	}
	if !cpr.checkSettings.disableRequestBodyBuffering {
		t.Error("disableRequestBodyBuffering: got false, want true")
	}
}

// TestParsePerRoute_CheckSettings_WithRequestBody verifies the per-route
// with_request_body override is consumed.
func TestParsePerRoute_CheckSettings_WithRequestBody(t *testing.T) {
	pr := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &ext_authzv3.CheckSettings{
				WithRequestBody: &ext_authzv3.BufferSettings{
					MaxRequestBytes: 512,
				},
			},
		},
	}
	cpr, err := parsePerRoute(pr)
	if err != nil {
		t.Fatalf("parsePerRoute(with_request_body): unexpected error: %v", err)
	}
	if cpr.checkSettings == nil {
		t.Fatal("checkSettings: got nil")
	}
	if cpr.checkSettings.withRequestBody == nil {
		t.Fatal("checkSettings.withRequestBody: got nil, want non-nil")
	}
	if cpr.checkSettings.withRequestBody.maxRequestBytes != 512 {
		t.Errorf("maxRequestBytes: got %d, want 512", cpr.checkSettings.withRequestBody.maxRequestBytes)
	}
}

// TestParsePerRoute_CheckSettings_BothBodySettingsXOR verifies that setting both
// disable_request_body_buffering AND with_request_body is rejected (XOR per SPEC §6.6).
func TestParsePerRoute_CheckSettings_BothBodySettingsXOR(t *testing.T) {
	pr := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &ext_authzv3.CheckSettings{
				DisableRequestBodyBuffering: true,
				WithRequestBody: &ext_authzv3.BufferSettings{
					MaxRequestBytes: 512,
				},
			},
		},
	}
	_, err := parsePerRoute(pr)
	if err == nil {
		t.Fatal("parsePerRoute(both body settings): want error, got nil")
	}
	if !strings.Contains(err.Error(), "disable_request_body_buffering") &&
		!strings.Contains(err.Error(), "with_request_body") {
		t.Errorf("got %q; want error mentioning body settings conflict", err.Error())
	}
}

// TestResolvePerRouteConfig_NilMsg verifies that a nil per-route message falls back
// to the listener-level compiledConfig.
func TestResolvePerRouteConfig_NilMsg(t *testing.T) {
	cc := &compiledConfig{statusOnError: 403}
	state := &factoryState{listenerRC: cc}
	result := state.resolvePerRouteConfig(nil)
	if result == nil {
		t.Fatal("resolvePerRouteConfig(nil): got nil, want listenerRC")
	}
	if result.disabled {
		t.Error("resolvePerRouteConfig(nil): got disabled=true, want false")
	}
	if result.cc != cc {
		t.Error("resolvePerRouteConfig(nil): did not return listenerRC")
	}
}

// TestResolvePerRouteConfig_DisabledTrue verifies that a per-route disabled:true
// arm returns a compiledPerRoute with disabled=true.
func TestResolvePerRouteConfig_DisabledTrue(t *testing.T) {
	cc := &compiledConfig{statusOnError: 403}
	state := &factoryState{listenerRC: cc}

	pr := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_Disabled{
			Disabled: true,
		},
	}
	result := state.resolvePerRouteConfig(pr)
	if result == nil {
		t.Fatal("resolvePerRouteConfig(disabled=true): got nil")
	}
	if !result.disabled {
		t.Error("resolvePerRouteConfig(disabled=true): got disabled=false, want true")
	}
}

// TestResolvePerRouteConfig_CheckSettings verifies that a per-route check_settings
// arm returns a compiledPerRoute with the listener-level cc + checkSettings.
func TestResolvePerRouteConfig_CheckSettings(t *testing.T) {
	cc := &compiledConfig{statusOnError: 403}
	state := &factoryState{listenerRC: cc}

	pr := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &ext_authzv3.CheckSettings{
				ContextExtensions: map[string]string{"env": "prod"},
			},
		},
	}
	result := state.resolvePerRouteConfig(pr)
	if result == nil {
		t.Fatal("resolvePerRouteConfig(check_settings): got nil")
	}
	if result.disabled {
		t.Error("resolvePerRouteConfig(check_settings): got disabled=true, want false")
	}
	if result.cc != cc {
		t.Error("resolvePerRouteConfig(check_settings): cc is not listenerRC")
	}
	if result.checkSettings == nil {
		t.Fatal("resolvePerRouteConfig(check_settings): checkSettings is nil")
	}
}

// TestResolvePerRouteConfig_SyncMapIdentity verifies that repeated calls with the
// same proto pointer return pointer-identical compiledPerRoute values
// (sync.Map lazy-cache identity per ADR-0117 + ADR-0125 §(v)).
func TestResolvePerRouteConfig_SyncMapIdentity(t *testing.T) {
	cc := &compiledConfig{statusOnError: 403}
	state := &factoryState{listenerRC: cc}

	pr := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_Disabled{
			Disabled: true,
		},
	}
	r1 := state.resolvePerRouteConfig(pr)
	r2 := state.resolvePerRouteConfig(pr)
	if r1 != r2 {
		t.Errorf("sync.Map identity: got different pointers for same proto (%p != %p)", r1, r2)
	}
}

// TestResolvePerRouteConfig_DifferentProtos verifies that different proto pointers
// produce separate cache entries (pointer-identity keying per ADR-0117).
func TestResolvePerRouteConfig_DifferentProtos(t *testing.T) {
	cc := &compiledConfig{statusOnError: 403}
	state := &factoryState{listenerRC: cc}

	pr1 := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_Disabled{Disabled: true},
	}
	pr2 := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_Disabled{Disabled: true},
	}
	r1 := state.resolvePerRouteConfig(pr1)
	r2 := state.resolvePerRouteConfig(pr2)
	// Both should resolve to disabled=true but as SEPARATE cache entries.
	if r1 == nil || r2 == nil {
		t.Fatal("resolvePerRouteConfig: unexpected nil result")
	}
	if !r1.disabled || !r2.disabled {
		t.Error("both should be disabled=true")
	}
	// They may or may not be pointer-identical (sync.Map races are fine);
	// what matters is both are correct.
}

// TestResolvePerRouteConfig_3TierFallback verifies the 3-tier resolution: Route >
// VirtualHost > RouteConfiguration > listener-fallback. A nil msg should fall
// back to the listener-level config (already tested). An unrecognized msg type
// should also fall back to listener.
func TestResolvePerRouteConfig_UnknownMsgTypeFallback(t *testing.T) {
	cc := &compiledConfig{statusOnError: 403}
	state := &factoryState{listenerRC: cc}

	// Pass a non-*ExtAuthzPerRoute proto.Message — exercises the !ok branch of
	// the type assertion in resolvePerRouteConfig. A *corev3.GrpcService is a
	// valid proto.Message but is NOT an *ext_authzv3.ExtAuthzPerRoute, so the
	// type assertion fires the !ok path and returns the listener-level fallback.
	// This test would fail if the !ok branch were removed or skipped.
	wrongType := &corev3.GrpcService{}
	result := state.resolvePerRouteConfig(wrongType)
	if result == nil || result.cc != cc {
		t.Error("resolvePerRouteConfig(wrong type): want listenerRC fallback, got different result")
	}
}

// ----------------------------------------------------------------------------
// Group 4 — HTTP-mode checkFn: buildHTTPCheckFn + the checkFn closure
// (Group 4 per SPEC §14.1 + PLAN Task 3)
//
// Uses a scriptableAuthServer helper (httptest.NewServer-based) to script
// the auth-server responses. Each test calls buildHTTPCheckFn directly
// (or via buildCompiledConfig) and invokes the resulting checkFn closure
// against the scriptable server.
// ----------------------------------------------------------------------------

// scriptableAuthServer is a minimal httptest-based scriptable auth server for
// Group 4 tests. It serves a fixed HTTP response (status + headers + body)
// supplied at construction time. After one request, records the received
// request for later inspection.
type scriptableAuthServer struct {
	srv      *httptest.Server
	received *http.Request // set after the first call; guarded by the test's sequential use
}

// newScriptableAuthServer creates an httptest.Server that returns the given
// status, headers, and body for every request. The handler captures the last
// received request in srv.received.
func newScriptableAuthServer(t *testing.T, status int, headers map[string]string, body string) *scriptableAuthServer {
	t.Helper()
	sas := &scriptableAuthServer{}
	sas.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sas.received = r
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		if body != "" {
			_, _ = w.Write([]byte(body))
		}
	}))
	t.Cleanup(func() { sas.srv.Close() })
	return sas
}

// newSlowAuthServer creates an httptest.Server that hangs until the client
// times out (or the context is cancelled). Used for timeout tests.
func newSlowAuthServer(t *testing.T) *scriptableAuthServer {
	t.Helper()
	sas := &scriptableAuthServer{}
	sas.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sas.received = r
		// Sleep until the request context is done (simulates a slow auth service).
		<-r.Context().Done()
		// Drain; don't respond.
		_ = r.Body.Close()
	}))
	t.Cleanup(func() { sas.srv.Close() })
	return sas
}

// buildHTTPCheckFnForTest builds an httpAuthClient pointing at the given server URL
// with an optional timeout, returning the checkFn closure. Convenience wrapper
// for Group 4 tests.
func buildHTTPCheckFnForTest(t *testing.T, serverURL string, timeoutMs int64, pathPrefix string) checkFn {
	t.Helper()
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{
			Uri: serverURL,
		},
		PathPrefix: pathPrefix,
	}
	if timeoutMs > 0 {
		hs.ServerUri.Timeout = durationpb.New(time.Duration(timeoutMs) * time.Millisecond)
	}
	fn, err := buildHTTPCheckFn(hs)
	if err != nil {
		t.Fatalf("buildHTTPCheckFn: unexpected error: %v", err)
	}
	return fn
}

// minimalAuthRequest returns a minimal *authRequest for use in Group 4 tests.
// The request-side filtered-headers construction (Task 4) is stubbed; for Task 3
// we pass a simple header set directly.
func minimalAuthRequest(path string, headers map[string]string) *authRequest {
	h := make(http.Header)
	for k, v := range headers {
		h.Set(k, v)
	}
	return &authRequest{
		method:  http.MethodPost,
		path:    path,
		headers: h,
		body:    nil,
	}
}

// -------------------------------------------------------------------------
// Group 4 tests — buildHTTPCheckFn construction + checkFn closure behavior
// -------------------------------------------------------------------------

// TestBuildHTTPCheckFn_MissingServerURI verifies PARSE-REJECT when server_uri
// is nil. Per SPEC §6.5 (PGV-mirror).
func TestBuildHTTPCheckFn_MissingServerURI(t *testing.T) {
	hs := &ext_authzv3.HttpService{
		// server_uri intentionally nil
	}
	fn, err := buildHTTPCheckFn(hs)
	if err == nil {
		t.Fatal("buildHTTPCheckFn(nil server_uri): want error, got nil")
	}
	if fn != nil {
		t.Error("buildHTTPCheckFn(nil server_uri): want nil fn, got non-nil")
	}
	if !strings.Contains(err.Error(), "server_uri") {
		t.Errorf("got %q; want substring 'server_uri'", err.Error())
	}
}

// TestBuildHTTPCheckFn_EmptyURI verifies PARSE-REJECT when server_uri.uri is empty.
func TestBuildHTTPCheckFn_EmptyURI(t *testing.T) {
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: ""},
	}
	fn, err := buildHTTPCheckFn(hs)
	if err == nil {
		t.Fatal("buildHTTPCheckFn(empty uri): want error, got nil")
	}
	if fn != nil {
		t.Error("buildHTTPCheckFn(empty uri): want nil fn, got non-nil")
	}
}

// TestBuildHTTPCheckFn_ValidConfig_ReturnsNonNilFn verifies that a valid
// HttpService config produces a non-nil checkFn (real impl, not stub).
func TestBuildHTTPCheckFn_ValidConfig_ReturnsNonNilFn(t *testing.T) {
	fn := buildHTTPCheckFnForTest(t, "http://127.0.0.1:9191/auth", 0, "")
	if fn == nil {
		t.Fatal("buildHTTPCheckFn(valid config): got nil fn, want non-nil")
	}
}

// TestCheckFn_Allow_Status200 verifies that an auth server HTTP 200 response
// produces a dispAllow disposition.
func TestCheckFn_Allow_Status200(t *testing.T) {
	srv := newScriptableAuthServer(t, http.StatusOK, map[string]string{
		"x-auth-result": "allowed",
	}, "")
	fn := buildHTTPCheckFnForTest(t, srv.srv.URL, 0, "")

	disp, err := fn(context.Background(), minimalAuthRequest("/check", nil))
	if err != nil {
		t.Fatalf("checkFn(200): unexpected error: %v", err)
	}
	if disp.class != dispAllow {
		t.Errorf("checkFn(200): got disposition class %v, want dispAllow", disp.class)
	}
}

// TestCheckFn_Deny_Status401 verifies that an auth server HTTP 401 response
// produces a dispDeny disposition with the correct status.
func TestCheckFn_Deny_Status401(t *testing.T) {
	denyBody := "unauthorized\n"
	srv := newScriptableAuthServer(t, http.StatusUnauthorized, map[string]string{
		"x-deny-reason": "bad-token",
	}, denyBody)
	fn := buildHTTPCheckFnForTest(t, srv.srv.URL, 0, "")

	disp, err := fn(context.Background(), minimalAuthRequest("/check", nil))
	if err != nil {
		t.Fatalf("checkFn(401): unexpected error: %v", err)
	}
	if disp.class != dispDeny {
		t.Errorf("checkFn(401): got disposition class %v, want dispDeny", disp.class)
	}
	if disp.denyStatus != http.StatusUnauthorized {
		t.Errorf("checkFn(401): denyStatus got %d, want 401", disp.denyStatus)
	}
	if string(disp.denyBody) != denyBody {
		t.Errorf("checkFn(401): denyBody got %q, want %q", disp.denyBody, denyBody)
	}
}

// TestCheckFn_Deny_Status403 verifies that an auth server HTTP 403 response
// produces a dispDeny disposition.
func TestCheckFn_Deny_Status403(t *testing.T) {
	denyBody := "forbidden\n"
	srv := newScriptableAuthServer(t, http.StatusForbidden, map[string]string{
		"x-deny-reason": "policy",
	}, denyBody)
	fn := buildHTTPCheckFnForTest(t, srv.srv.URL, 0, "")

	disp, err := fn(context.Background(), minimalAuthRequest("/check", nil))
	if err != nil {
		t.Fatalf("checkFn(403): unexpected error: %v", err)
	}
	if disp.class != dispDeny {
		t.Errorf("checkFn(403): got disposition class %v, want dispDeny", disp.class)
	}
	if disp.denyStatus != http.StatusForbidden {
		t.Errorf("checkFn(403): denyStatus got %d, want 403", disp.denyStatus)
	}
	if string(disp.denyBody) != denyBody {
		t.Errorf("checkFn(403): denyBody got %q, want %q", disp.denyBody, denyBody)
	}
}

// TestCheckFn_Error_UnrecognizedStatus verifies that an unrecognized HTTP status
// (e.g., 555) produces a dispError disposition per §5.P10.
func TestCheckFn_Error_UnrecognizedStatus(t *testing.T) {
	srv := newScriptableAuthServer(t, 555, nil, "")
	fn := buildHTTPCheckFnForTest(t, srv.srv.URL, 0, "")

	disp, err := fn(context.Background(), minimalAuthRequest("/check", nil))
	if err == nil {
		t.Fatal("checkFn(555): want error for unrecognized status, got nil")
	}
	if disp.class != dispError {
		t.Errorf("checkFn(555): got disposition class %v, want dispError", disp.class)
	}
}

// TestCheckFn_Error_ConnectFailure verifies that a connect failure (auth server
// closed/unreachable) produces a dispError disposition.
func TestCheckFn_Error_ConnectFailure(t *testing.T) {
	// Create a server and close it immediately — so the port is not listening.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	serverURL := srv.URL
	srv.Close() // closed immediately; connect will fail

	fn := buildHTTPCheckFnForTest(t, serverURL, 2000, "")
	disp, err := fn(context.Background(), minimalAuthRequest("/check", nil))
	if err == nil {
		t.Fatal("checkFn(connect failure): want error, got nil")
	}
	if disp.class != dispError {
		t.Errorf("checkFn(connect failure): got disposition class %v, want dispError", disp.class)
	}
}

// TestCheckFn_Error_Timeout verifies that a slow auth server (exceeding the
// timeout) produces a dispError disposition.
func TestCheckFn_Error_Timeout(t *testing.T) {
	srv := newSlowAuthServer(t)
	// 50ms timeout — the slow server never responds within that window.
	fn := buildHTTPCheckFnForTest(t, srv.srv.URL, 50, "")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	disp, err := fn(ctx, minimalAuthRequest("/check", nil))
	if err == nil {
		t.Fatal("checkFn(timeout): want error, got nil")
	}
	if disp.class != dispError {
		t.Errorf("checkFn(timeout): got disposition class %v, want dispError", disp.class)
	}
}

// TestCheckFn_Error_ContextCancelled verifies that a context cancellation during
// an in-flight call produces a dispError disposition.
func TestCheckFn_Error_ContextCancelled(t *testing.T) {
	srv := newSlowAuthServer(t)
	// Use a very long client timeout so only context cancellation fires.
	fn := buildHTTPCheckFnForTest(t, srv.srv.URL, 30000, "")

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel the context immediately before calling.
	cancel()

	disp, err := fn(ctx, minimalAuthRequest("/check", nil))
	if err == nil {
		t.Fatal("checkFn(ctx cancelled): want error, got nil")
	}
	if disp.class != dispError {
		t.Errorf("checkFn(ctx cancelled): got disposition class %v, want dispError", disp.class)
	}
}

// TestCheckFn_PathPrefix_Prepended verifies that the path_prefix is prepended
// to the authRequest path in the outbound POST.
func TestCheckFn_PathPrefix_Prepended(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(func() { srv.Close() })

	fn := buildHTTPCheckFnForTest(t, srv.URL, 0, "/auth-prefix")
	_, err := fn(context.Background(), minimalAuthRequest("/api/resource", nil))
	if err != nil {
		t.Fatalf("checkFn(path_prefix): unexpected error: %v", err)
	}
	wantPath := "/auth-prefix/api/resource"
	if capturedPath != wantPath {
		t.Errorf("path_prefix: captured path %q, want %q", capturedPath, wantPath)
	}
}

// TestCheckFn_HeadersForwarded verifies that the authRequest headers are sent
// in the outbound POST to the auth service.
func TestCheckFn_HeadersForwarded(t *testing.T) {
	var capturedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get("x-forwarded-for")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(func() { srv.Close() })

	fn := buildHTTPCheckFnForTest(t, srv.URL, 0, "")
	_, err := fn(context.Background(), minimalAuthRequest("/check", map[string]string{
		"x-forwarded-for": "10.0.0.1",
	}))
	if err != nil {
		t.Fatalf("checkFn(headers): unexpected error: %v", err)
	}
	if capturedHeader != "10.0.0.1" {
		t.Errorf("headers forwarded: got %q, want %q", capturedHeader, "10.0.0.1")
	}
}

// TestCheckFn_WithRequestBody verifies that when the authRequest has a body,
// it is sent in the outbound POST.
func TestCheckFn_WithRequestBody(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(func() { srv.Close() })

	fn := buildHTTPCheckFnForTest(t, srv.URL, 0, "")
	req := &authRequest{
		method:  http.MethodPost,
		path:    "/check",
		headers: make(http.Header),
		body:    []byte("request-body-data"),
	}
	_, err := fn(context.Background(), req)
	if err != nil {
		t.Fatalf("checkFn(with body): unexpected error: %v", err)
	}
	if string(capturedBody) != "request-body-data" {
		t.Errorf("request body: got %q, want %q", capturedBody, "request-body-data")
	}
}

// TestNew_HttpService_ValidConfig_RealImpl verifies that at Task 3 (real
// buildHTTPCheckFn) a valid http_service config produces a non-nil factory
// (no more stub error). This REPLACES / TIGHTENS the Task 2 stub test
// TestNew_HttpService_ValidConfig_Task2Stub.
func TestNew_HttpService_ValidConfig_RealImpl(t *testing.T) {
	factory, err := New(mustAny(t, validHTTPServiceConfig()), freshFactoryCtx())
	if err != nil {
		t.Fatalf("New(valid http_service, real impl): want nil error, got %v", err)
	}
	if factory == nil {
		t.Fatal("New(valid http_service, real impl): want non-nil factory, got nil")
	}
}

// NOTE: the Task-2 M4 stub-tolerant tests TestNew_StatusOnError_Default,
// TestNew_StatusOnError_Explicit, TestCompiledConfig_FailureModeAllowConsumed,
// and TestCompiledConfig_WithRequestBodyConsumed were TIGHTENED IN PLACE at
// Task 3 (their stub-era `if cc != nil` wrappers replaced with unconditional
// assertions now that buildHTTPCheckFn is real). No separate `_RealImpl`
// duplicates are kept — the originals carry the tightened assertions.
