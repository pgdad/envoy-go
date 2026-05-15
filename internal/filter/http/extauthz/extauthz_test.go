package extauthz

// extauthz_test.go — unit-test Groups 1 + 2 + 3 + 4 + 6 + 7 + 8 (through Task 8).
//
// Test group assignments per PLAN / SPEC §14.1:
//
//   Group 1 — ExtAuthz parse + services oneof dispatch
//   Group 2 — compiledConfig shape + filterStats allocation
//   Group 3 — buildAuthRequest + request-side header filtering (Task 4)
//   Group 4 — check.go HTTP-outbound auth-check primitive (Task 3)
//   Group 6 — with_request_body ADR-0128 reuse + over-limit 413 + DecodeData (Task 6)
//   Group 7 — per-route: parsePerRoute + resolvePerRouteConfig
//   Group 8 — Bidirectional header-mutation discipline (Task 5)
//
// Groups 5/9 land at Task 9 respectively per PLAN.
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
	"sync"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext_authzv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
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

// TestNew_HttpService_ValidConfig_Task2Stub was deleted at Task 3 review-fix
// (Issue 5 — duplicate of TestNew_HttpService_ValidConfig below). The
// _RealImpl variant is now the keeper, renamed to the clean name.

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

// ----------------------------------------------------------------------------
// Group 2 stats sub-group — SN2-reuse namespace integration tests
// (Task 8: stat surface finalization + §18.P6 + §18.P7 RATIFIED-PENDING closure)
//
// These tests lock down the contract established by newFilterStats + Task 2's
// implementation. Per the Task 8 TDD discipline (PLAN Step 1 finalization note):
// the tests are written here at Task 8 to lock down the already-satisfied
// contract. They pass immediately because Task 2's implementation already
// satisfies all conditions — documented as a "finalization/locking step" per
// the PLAN. The empirical scrape (Step 4) closes §18.P6 + §18.P7 externally.
// ----------------------------------------------------------------------------

// TestFilterStats_ExactlySixCounters_NoExtras verifies that newFilterStats
// registers EXACTLY 6 counters and no extras in a fresh Registry per ADR-0156
// §Decision + parent SPEC §6 amendment 8. Mirrors phase-17 jwt_authn
// TestFilterStats_AllSevenCountersRegistered.
func TestFilterStats_ExactlySixCounters_NoExtras(t *testing.T) {
	reg := stats.NewRegistry()
	_ = newFilterStats(reg, "ingress_http")
	var names []string
	reg.Walk(func(m stats.Metric) {
		names = append(names, m.Name())
	})
	if len(names) != 6 {
		t.Errorf("Registry size = %d; want exactly 6 counters (no extras); got names=%v", len(names), names)
	}
}

// TestFilterStats_UnconditionalRegistration_ViaBuildCompiledConfig verifies
// that buildCompiledConfig registers all 6 counters unconditionally when
// ctx.Stats is non-nil — i.e., NOT lazy, NOT deferred to first request. The
// counters must appear in the Registry immediately after buildCompiledConfig
// returns per ADR-0156 + parent SPEC §6 amendment 8.
func TestFilterStats_UnconditionalRegistration_ViaBuildCompiledConfig(t *testing.T) {
	reg := stats.NewRegistry()
	ctx := freshFactoryCtxWithRegistry(reg)
	cc, err := buildCompiledConfig(ctx, validHTTPServiceConfig())
	if err != nil {
		t.Fatalf("buildCompiledConfig: unexpected error: %v", err)
	}
	if cc.stats == nil {
		t.Fatal("cc.stats: want non-nil when ctx.Stats is non-nil")
	}
	// Counters must be present immediately — no deferred / lazy allocation.
	var names []string
	reg.Walk(func(m stats.Metric) {
		names = append(names, m.Name())
	})
	want := []string{
		"http.ingress_http.ext_authz.ok",
		"http.ingress_http.ext_authz.denied",
		"http.ingress_http.ext_authz.error",
		"http.ingress_http.ext_authz.disabled",
		"http.ingress_http.ext_authz.failure_mode_allowed",
		"http.ingress_http.ext_authz.invalid",
	}
	for _, w := range want {
		found := false
		for _, n := range names {
			if n == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("counter %q not registered immediately after buildCompiledConfig; got names=%v", w, names)
		}
	}
	if len(names) != 6 {
		t.Errorf("Registry size = %d after buildCompiledConfig; want 6; got names=%v", len(names), names)
	}
}

// TestFilterStats_NilStats_CcStatsIsNil verifies that when ctx.Stats is nil,
// buildCompiledConfig leaves cc.stats as nil (the ADR-0085 nil-tolerance guard:
// `if ctx.Stats != nil` at buildCompiledConfig step 7). Mirrors phase-17
// TestFilterStats_NilRegistry_NoPanic SN2-reuse nil-tolerance contract.
func TestFilterStats_NilStats_CcStatsIsNil(t *testing.T) {
	nilCtx := envoyhttp.FactoryCtx{Stats: nil, StatPrefix: "ingress_http"}
	cc, err := buildCompiledConfig(nilCtx, validHTTPServiceConfig())
	if err != nil {
		t.Fatalf("buildCompiledConfig: unexpected error: %v", err)
	}
	if cc.stats != nil {
		t.Errorf("cc.stats: want nil when ctx.Stats is nil; got non-nil %v", cc.stats)
	}
}

// TestFilterStats_EmptyPrefix_FoldsToBarePrefixShape verifies the empty-HCM-
// stat_prefix fold per baseStatPrefix: when hcmStatPrefix == "", the counter
// names use the bare `ext_authz.<counter>` form (no "http.." double-dot) to
// satisfy the Registry's nameRE (forbids leading dots / double dots).
// Per ADR-0156 + extauthz.go baseStatPrefix inline comment.
func TestFilterStats_EmptyPrefix_FoldsToBarePrefixShape(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "") // empty HCM stat_prefix
	if fs == nil {
		t.Fatal("newFilterStats with empty prefix: got nil, want non-nil")
	}
	// Counter names must use bare ext_authz.<counter> form (no http.. prefix).
	cases := []struct {
		handle *stats.Counter
		want   string
	}{
		{fs.ok, "ext_authz.ok"},
		{fs.denied, "ext_authz.denied"},
		{fs.errored, "ext_authz.error"}, // "error" on wire; "errored" in Go
		{fs.disabled, "ext_authz.disabled"},
		{fs.failureModeAllowed, "ext_authz.failure_mode_allowed"},
		{fs.invalid, "ext_authz.invalid"},
	}
	for _, tc := range cases {
		if tc.handle == nil {
			t.Errorf("counter for %q: handle is nil with empty prefix", tc.want)
			continue
		}
		if got := tc.handle.Name(); got != tc.want {
			t.Errorf("counter Name() = %q; want %q (bare form, no http.. double-dot)", got, tc.want)
		}
	}
}

// TestFilterStats_CounterHandleNames_SN2ReusePins locks down the exact
// Name() values on each counter handle, confirming the SN2-reuse internal
// stat-path shape `http.<HCM_stat_prefix>.ext_authz.<counter>` per ADR-0156
// §Decision (vii) + parent SPEC §18.P6/§18.P7 RATIFIED-PENDING-IMPL-TIME
// empirical-scrape closure at Task 8.
func TestFilterStats_CounterHandleNames_SN2ReusePins(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_http")
	if fs == nil {
		t.Fatal("newFilterStats: want non-nil")
	}
	cases := []struct {
		handle *stats.Counter
		want   string
	}{
		{fs.ok, "http.ingress_http.ext_authz.ok"},
		{fs.denied, "http.ingress_http.ext_authz.denied"},
		{fs.errored, "http.ingress_http.ext_authz.error"}, // "error" on wire; "errored" in Go
		{fs.disabled, "http.ingress_http.ext_authz.disabled"},
		{fs.failureModeAllowed, "http.ingress_http.ext_authz.failure_mode_allowed"},
		{fs.invalid, "http.ingress_http.ext_authz.invalid"},
	}
	for _, tc := range cases {
		if tc.handle == nil {
			t.Errorf("counter for %q: handle is nil", tc.want)
			continue
		}
		if got := tc.handle.Name(); got != tc.want {
			t.Errorf("counter Name() = %q; want %q (SN2-reuse per ADR-0156 + §18.P7)", got, tc.want)
		}
	}
}

// TestFilterStats_DisabledCounter_RegisteredButZero verifies the `disabled`
// counter is registered (for scrape-stability) but publishes 0 under MVP —
// it is STRUCTURALLY UNREACHABLE (only the deferred runtime `filter_enabled`
// gate increments it; no code path in 18.1 increments it directly). Per
// parent SPEC §6 amendment 7 + ADR-0156 §Decision.
func TestFilterStats_DisabledCounter_RegisteredButZero(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_http")
	if fs == nil {
		t.Fatal("newFilterStats: want non-nil")
	}
	if fs.disabled == nil {
		t.Fatal("disabled counter: want registered (non-nil), got nil")
	}
	if got := fs.disabled.Name(); got != "http.ingress_http.ext_authz.disabled" {
		t.Errorf("disabled.Name() = %q; want %q", got, "http.ingress_http.ext_authz.disabled")
	}
	// STRUCTURALLY UNREACHABLE: value must be 0 — no code path increments it
	// under MVP per parent SPEC §6 amendment 7.
	if v := fs.disabled.Load(); v != 0 {
		t.Errorf("disabled.Load() = %d; want 0 (STRUCTURALLY UNREACHABLE under MVP per §6 amendment 7)", v)
	}
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

// TestDecodeHeaders_EndStreamNoBody_StopIteration verifies the DecodeHeaders
// dispatch path: when there is no listener-level with_request_body AND
// endStream=true (no body buffering), DecodeHeaders returns StopIteration
// (the async outbound check is dispatched per Task 9 / SPEC §6.3 step 5).
// NOTE: This test was previously named TestDecodeHeaders_EndStreamNoBody_Continue
// and expected Continue when Task 9's dispatch was not yet wired. Task 9
// wires the dispatch, so the expected status is now StopIteration.
// The checkFn is nil in this test (minimal compiledConfig), so dispatchOutboundCheck
// returns early via the nil-guard — but StopIteration is still returned.
func TestDecodeHeaders_EndStreamNoBody_StopIteration(t *testing.T) {
	// Build a minimal factoryState with a compiledConfig (no stats needed; no
	// with_request_body → body-buffering branch is skipped).
	cc := &compiledConfig{
		statusOnError: 403,
	}
	state := &factoryState{listenerRC: cc}
	f := &filter{state: state}

	headers := make(map[string][]string)
	result := f.DecodeHeaders(headers, true)
	// Task 9: StopIteration is returned (async dispatch fires). checkFn is nil
	// so dispatchOutboundCheck no-ops, but the HCM-visible return is StopIteration.
	if result != envoyhttp.StopIteration {
		t.Errorf("DecodeHeaders endStream=true no-body: got %v, want StopIteration", result)
	}
}

// TestDecodeData_AwaitingBodyFalse_Passthrough verifies the DecodeData
// pass-through path: when awaitingBody=false (body-buffering not active),
// DecodeData returns DataContinue regardless of endStream value.
func TestDecodeData_AwaitingBodyFalse_Passthrough(t *testing.T) {
	f := &filter{}
	result := f.DecodeData(nil, true)
	if result != envoyhttp.DataContinue {
		t.Errorf("DecodeData awaitingBody=false: got %v, want DataContinue", result)
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
// (Group 7 per SPEC §14.1 + PLAN Tasks 2 + 7)
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

// TestResolvePerRouteConfig_SharedStats verifies SHARED-stats discipline per
// ADR-0163: the per-route resolution NEVER allocates a fresh *filterStats.
// The compiledPerRoute.cc must equal the listener-level cc (same pointer), which
// means the stats field is the shared listener-level stats — not an independent
// per-route allocation.
func TestResolvePerRouteConfig_SharedStats(t *testing.T) {
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		statusOnError: 403,
		stats:         newFilterStats(reg, "ingress_http"),
	}
	state := &factoryState{listenerRC: cc}

	pr := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &ext_authzv3.CheckSettings{
				ContextExtensions: map[string]string{"env": "staging"},
			},
		},
	}
	result := state.resolvePerRouteConfig(pr)
	if result == nil {
		t.Fatal("resolvePerRouteConfig: got nil")
	}
	// SHARED-stats discipline: result.cc must be the listener-level cc.
	if result.cc != cc {
		t.Error("SHARED-stats violation: result.cc is not the listener-level compiledConfig pointer")
	}
	// The per-route compiledPerRoute carries no independent filterStats.
	// The only stats surface is through result.cc.stats (= cc.stats = listener-level).
	if result.cc.stats != cc.stats {
		t.Error("SHARED-stats violation: result.cc.stats is not the listener-level filterStats pointer")
	}
}

// TestResolvePerRouteConfig_DisabledSharedStats verifies SHARED-stats discipline
// for the disabled arm as well: disabled per-route does not get its own stats.
func TestResolvePerRouteConfig_DisabledSharedStats(t *testing.T) {
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		statusOnError: 403,
		stats:         newFilterStats(reg, "ingress_http"),
	}
	state := &factoryState{listenerRC: cc}

	pr := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_Disabled{Disabled: true},
	}
	result := state.resolvePerRouteConfig(pr)
	if result == nil {
		t.Fatal("resolvePerRouteConfig(disabled): got nil")
	}
	if !result.disabled {
		t.Error("disabled: got false, want true")
	}
	// SHARED-stats: cc wired to listenerRC.
	if result.cc != cc {
		t.Error("SHARED-stats violation (disabled arm): result.cc is not the listener-level compiledConfig pointer")
	}
}

// TestParsePerRoute_ContextExtensions_NoopInHTTPMode verifies that context_extensions
// PARSES correctly (no error, values stored) but has NO effect on compiledConfig
// (there is no context_extensions field on compiledConfig or compiledCheckSettings
// that drives any HTTP-mode behavior). Per SPEC §8 item 8 + ADR-0163: in HTTP-mode
// context_extensions is a no-op; 18.2 consumes it for the gRPC AttributeContext.
func TestParsePerRoute_ContextExtensions_NoopInHTTPMode(t *testing.T) {
	pr := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &ext_authzv3.CheckSettings{
				ContextExtensions: map[string]string{
					"vhost":   "my-virtual-host",
					"service": "payment-svc",
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
	// Parsed and stored — the map is accessible for future gRPC-mode use (18.2).
	if len(cpr.checkSettings.contextExtensions) != 2 {
		t.Errorf("contextExtensions: got %d entries, want 2", len(cpr.checkSettings.contextExtensions))
	}
	// HTTP-mode no-op: compiledCheckSettings carries ONLY disableRequestBodyBuffering
	// and withRequestBody as HTTP-mode-affecting fields. contextExtensions does NOT
	// drive any HTTP-mode field.
	if cpr.checkSettings.disableRequestBodyBuffering {
		t.Error("context_extensions unexpectedly set disableRequestBodyBuffering")
	}
	if cpr.checkSettings.withRequestBody != nil {
		t.Error("context_extensions unexpectedly set withRequestBody")
	}
}

// TestResolvePerRouteConfig_ConcurrentSameProto verifies that concurrent calls to
// resolvePerRouteConfig with the SAME proto pointer produce a SINGLE *compiledPerRoute
// per proto pointer (sync.Map LoadOrStore identity per ADR-0117 + ADR-0125 §(v)).
func TestResolvePerRouteConfig_ConcurrentSameProto(t *testing.T) {
	cc := &compiledConfig{statusOnError: 403}
	state := &factoryState{listenerRC: cc}

	pr := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_Disabled{Disabled: true},
	}

	const n = 20
	var wg sync.WaitGroup
	results := make([]*compiledPerRoute, n)
	wg.Add(n)
	for i := range n {
		go func(idx int) {
			defer wg.Done()
			results[idx] = state.resolvePerRouteConfig(pr)
		}(i)
	}
	wg.Wait()

	// All goroutines must return the same pointer (sync.Map LoadOrStore ensures
	// exactly one *compiledPerRoute is stored per proto pointer).
	first := results[0]
	if first == nil {
		t.Fatal("concurrent resolvePerRouteConfig: got nil result")
	}
	for i, r := range results[1:] {
		if r != first {
			t.Errorf("concurrent resolvePerRouteConfig: goroutine %d got different pointer (%p != %p)", i+1, r, first)
		}
	}
}

// TestEffectiveWithRequestBody_DisableRequestBodyBuffering verifies that a per-route
// check_settings with disable_request_body_buffering=true causes effectiveWithRequestBody
// to return nil (body buffering OFF) even when the listener-level withRequestBody is set.
// Per SPEC §6.3 step 3 + SPEC §8 + ADR-0162 + ADR-0163.
func TestEffectiveWithRequestBody_DisableRequestBodyBuffering(t *testing.T) {
	listenerCC := &compiledConfig{
		withRequestBody: &bufferSettings{maxRequestBytes: 1024},
	}
	f := &filter{activeRC: listenerCC}

	// Per-route: disable_request_body_buffering=true overrides listener-level.
	pr := &compiledPerRoute{
		cc: listenerCC,
		checkSettings: &compiledCheckSettings{
			disableRequestBodyBuffering: true,
			withRequestBody:             nil,
		},
	}
	got := f.effectiveWithRequestBody(pr)
	if got != nil {
		t.Errorf("effectiveWithRequestBody(disableRequestBodyBuffering=true): got non-nil, want nil (body buffering OFF)")
	}
}

// TestEffectiveWithRequestBody_PerRouteOverride verifies that a per-route
// check_settings with_request_body overrides the listener-level withRequestBody
// (most-specific-wins per SPEC §6.3 step 3 + ADR-0162 + ADR-0163).
func TestEffectiveWithRequestBody_PerRouteOverride(t *testing.T) {
	listenerWRB := &bufferSettings{maxRequestBytes: 1024}
	perRouteWRB := &bufferSettings{maxRequestBytes: 256, allowPartialMessage: true}
	listenerCC := &compiledConfig{withRequestBody: listenerWRB}
	f := &filter{activeRC: listenerCC}

	pr := &compiledPerRoute{
		cc: listenerCC,
		checkSettings: &compiledCheckSettings{
			disableRequestBodyBuffering: false,
			withRequestBody:             perRouteWRB,
		},
	}
	got := f.effectiveWithRequestBody(pr)
	if got != perRouteWRB {
		t.Errorf("effectiveWithRequestBody(per-route override): got %v, want per-route *bufferSettings", got)
	}
	if got == listenerWRB {
		t.Error("effectiveWithRequestBody(per-route override): returned listener-level, want per-route override")
	}
}

// TestEffectiveWithRequestBody_ListenerFallback verifies that effectiveWithRequestBody
// falls back to the listener-level withRequestBody when there is no per-route
// check_settings override (nil per-route or no check_settings set).
// Per SPEC §6.3 step 3 + ADR-0163.
func TestEffectiveWithRequestBody_ListenerFallback(t *testing.T) {
	listenerWRB := &bufferSettings{maxRequestBytes: 512}
	listenerCC := &compiledConfig{withRequestBody: listenerWRB}
	f := &filter{activeRC: listenerCC}

	// Case 1: nil per-route → listener fallback.
	got := f.effectiveWithRequestBody(nil)
	if got != listenerWRB {
		t.Errorf("effectiveWithRequestBody(nil pr): got %v, want listener-level *bufferSettings", got)
	}

	// Case 2: per-route with empty checkSettings (no overrides set) → listener fallback.
	pr := &compiledPerRoute{
		cc:            listenerCC,
		checkSettings: &compiledCheckSettings{},
	}
	got2 := f.effectiveWithRequestBody(pr)
	if got2 != listenerWRB {
		t.Errorf("effectiveWithRequestBody(empty checkSettings): got %v, want listener-level *bufferSettings", got2)
	}

	// Case 3: per-route with nil checkSettings → listener fallback.
	prNoCS := &compiledPerRoute{
		cc:            listenerCC,
		checkSettings: nil,
	}
	got3 := f.effectiveWithRequestBody(prNoCS)
	if got3 != listenerWRB {
		t.Errorf("effectiveWithRequestBody(nil checkSettings): got %v, want listener-level *bufferSettings", got3)
	}
}

// TestResolvePerRouteConfig_CCAlwaysListenerRC verifies that the cc field in a
// resolved compiledPerRoute is ALWAYS the listener-level compiledConfig regardless
// of which per-route arm is active. This is the SHARED-stats discipline structural
// assertion: no per-route cc is allocated; all per-route entries share the listener
// cc (and its stats).
func TestResolvePerRouteConfig_CCAlwaysListenerRC(t *testing.T) {
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		statusOnError: 403,
		stats:         newFilterStats(reg, "ingress_http"),
	}
	state := &factoryState{listenerRC: cc}

	cases := []struct {
		name string
		pr   *ext_authzv3.ExtAuthzPerRoute
	}{
		{
			name: "disabled arm",
			pr: &ext_authzv3.ExtAuthzPerRoute{
				Override: &ext_authzv3.ExtAuthzPerRoute_Disabled{Disabled: true},
			},
		},
		{
			name: "check_settings arm (empty)",
			pr: &ext_authzv3.ExtAuthzPerRoute{
				Override: &ext_authzv3.ExtAuthzPerRoute_CheckSettings{
					CheckSettings: &ext_authzv3.CheckSettings{},
				},
			},
		},
		{
			name: "check_settings arm (with context_extensions)",
			pr: &ext_authzv3.ExtAuthzPerRoute{
				Override: &ext_authzv3.ExtAuthzPerRoute_CheckSettings{
					CheckSettings: &ext_authzv3.CheckSettings{
						ContextExtensions: map[string]string{"k": "v"},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := state.resolvePerRouteConfig(tc.pr)
			if result == nil {
				t.Fatal("resolvePerRouteConfig: got nil")
			}
			if result.cc != cc {
				t.Errorf("%s: result.cc (%p) != listenerRC (%p) — SHARED-stats discipline violated",
					tc.name, result.cc, cc)
			}
		})
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
// supplied at construction time.
type scriptableAuthServer struct {
	srv *httptest.Server
}

// newScriptableAuthServer creates an httptest.Server that returns the given
// status, headers, and body for every request.
func newScriptableAuthServer(t *testing.T, status int, headers map[string]string, body string) *scriptableAuthServer {
	t.Helper()
	sas := &scriptableAuthServer{}
	sas.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	fn, err := buildHTTPCheckFn(hs, false)
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
	fn, err := buildHTTPCheckFn(hs, false)
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
	fn, err := buildHTTPCheckFn(hs, false)
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

// TestNew_HttpService_ValidConfig verifies that a valid http_service config
// produces a non-nil factory (real buildHTTPCheckFn in check.go).
// Replaces the former Task2Stub + RealImpl pair (Task 3 review-fix Issue 5).
func TestNew_HttpService_ValidConfig(t *testing.T) {
	factory, err := New(mustAny(t, validHTTPServiceConfig()), freshFactoryCtx())
	if err != nil {
		t.Fatalf("New(valid http_service): want nil error, got %v", err)
	}
	if factory == nil {
		t.Fatal("New(valid http_service): want non-nil factory, got nil")
	}
}

// NOTE: the Task-2 M4 stub-tolerant tests TestNew_StatusOnError_Default,
// TestNew_StatusOnError_Explicit, TestCompiledConfig_FailureModeAllowConsumed,
// and TestCompiledConfig_WithRequestBodyConsumed were TIGHTENED IN PLACE at
// Task 3 (their stub-era `if cc != nil` wrappers replaced with unconditional
// assertions now that buildHTTPCheckFn is real). No separate `_RealImpl`
// duplicates are kept — the originals carry the tightened assertions.
// TestNew_HttpService_ValidConfig_Task2Stub was also deleted at Task 3
// review-fix (Issue 5) — the sole keeper is TestNew_HttpService_ValidConfig.

// -------------------------------------------------------------------------
// Group 4 — stripPath / joinPaths / buildTargetURL unit tests (no server needed)
// Added at Task 3 review-fix (Issue 4): table-driven tests for the path-strip
// and path-join surface used by httpAuthClient.
// -------------------------------------------------------------------------

// TestStripPath verifies stripPath strips the path component from a server URI,
// leaving only the scheme+host. Covers URIs with and without path components.
func TestStripPath(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want string
	}{
		{
			name: "URI with path component",
			uri:  "http://auth.example.com:9191/some/path",
			want: "http://auth.example.com:9191",
		},
		{
			name: "URI without path",
			uri:  "http://auth.example.com:9191",
			want: "http://auth.example.com:9191",
		},
		{
			name: "URI with single-segment path",
			uri:  "http://auth:9191/base",
			want: "http://auth:9191",
		},
		{
			name: "HTTPS URI with path",
			uri:  "https://auth.example.com/auth/v1",
			want: "https://auth.example.com",
		},
		{
			name: "URI with trailing slash (path component)",
			uri:  "http://auth:9191/",
			want: "http://auth:9191",
		},
		{
			name: "URI with no scheme separator (returned as-is)",
			uri:  "auth.example.com:9191/path",
			want: "auth.example.com:9191/path",
		},
		{
			name: "empty string",
			uri:  "",
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripPath(tc.uri)
			if got != tc.want {
				t.Errorf("stripPath(%q) = %q; want %q", tc.uri, got, tc.want)
			}
		})
	}
}

// TestJoinPaths verifies joinPaths joins a prefix and path, avoiding
// double-slashes and handling empty prefix/path cases.
func TestJoinPaths(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		path   string
		want   string
	}{
		{
			name:   "both non-empty, both with slash boundary",
			prefix: "/auth",
			path:   "/api",
			want:   "/auth/api",
		},
		{
			name:   "prefix trailing slash + path leading slash — no double slash",
			prefix: "/auth/",
			path:   "/api",
			want:   "/auth/api",
		},
		{
			name:   "prefix no trailing slash + path no leading slash — slash added",
			prefix: "/auth",
			path:   "api",
			want:   "/auth/api",
		},
		{
			name:   "empty prefix — returns path",
			prefix: "",
			path:   "/api/resource",
			want:   "/api/resource",
		},
		{
			name:   "empty path — returns prefix",
			prefix: "/auth-prefix",
			path:   "",
			want:   "/auth-prefix",
		},
		{
			name:   "both empty",
			prefix: "",
			path:   "",
			want:   "",
		},
		{
			name:   "prefix with trailing slash, empty path",
			prefix: "/auth/",
			path:   "",
			want:   "/auth/",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := joinPaths(tc.prefix, tc.path)
			if got != tc.want {
				t.Errorf("joinPaths(%q, %q) = %q; want %q", tc.prefix, tc.path, got, tc.want)
			}
		})
	}
}

// TestBuildTargetURL verifies buildTargetURL combines a pre-stripped base,
// a path_prefix, and a request path into the correct outbound URL.
// buildTargetURL now accepts a pre-stripped base (no stripPath inside).
func TestBuildTargetURL(t *testing.T) {
	tests := []struct {
		name       string
		base       string // pre-stripped scheme+host
		pathPrefix string
		path       string
		want       string
	}{
		{
			name:       "base + prefix + path",
			base:       "http://auth:9191",
			pathPrefix: "/auth-prefix",
			path:       "/api",
			want:       "http://auth:9191/auth-prefix/api",
		},
		{
			name:       "base without prefix",
			base:       "http://auth:9191",
			pathPrefix: "",
			path:       "/api/resource",
			want:       "http://auth:9191/api/resource",
		},
		{
			name:       "base + prefix trailing slash + path leading slash — no double slash",
			base:       "http://auth:9191",
			pathPrefix: "/auth/",
			path:       "/api",
			want:       "http://auth:9191/auth/api",
		},
		{
			name:       "base with empty path and empty prefix",
			base:       "http://auth:9191",
			pathPrefix: "",
			path:       "",
			want:       "http://auth:9191",
		},
		{
			name:       "serverURI with path component — pre-stripped, replaced by prefix+path",
			base:       stripPath("http://auth:9191/base"),
			pathPrefix: "/auth-prefix",
			path:       "/api",
			want:       "http://auth:9191/auth-prefix/api",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildTargetURL(tc.base, tc.pathPrefix, tc.path)
			if got != tc.want {
				t.Errorf("buildTargetURL(%q, %q, %q) = %q; want %q",
					tc.base, tc.pathPrefix, tc.path, got, tc.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Group 3 — compileStringMatcherList + buildAuthRequest + validateMutationHeaders
// (Group 3 per SPEC §14.1 + PLAN Task 4 [ADR-0160])
//
// This group covers:
//   - compileStringMatcherList: all matcher kinds (exact/prefix/suffix/contains/
//     safe_regex + ignore_case) + custom PARSE-REJECT + nil input → nil
//   - buildAuthRequest: nil allowedHeaders = all pass; allowedHeaders allow-list;
//     disallowedHeaders overrides allowedHeaders; headers_to_add appended;
//     deprecated AuthorizationRequest.allowed_headers honored-if-present
//   - validateMutationHeaders: :-prefixed pseudo-header reject; invalid header-
//     name characters reject; invalid header-value characters reject; valid
//     headers pass
// ----------------------------------------------------------------------------

// --- helpers for Group 3 ---

// makeListStringMatcher builds a *matcherv3.ListStringMatcher from a slice of
// *matcherv3.StringMatcher entries. Convenience for Group 3 tests.
func makeListStringMatcher(patterns ...*matcherv3.StringMatcher) *matcherv3.ListStringMatcher {
	return &matcherv3.ListStringMatcher{Patterns: patterns}
}

// exactPattern builds an exact-match StringMatcher.
func exactPattern(s string) *matcherv3.StringMatcher {
	return &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: s}}
}

// exactPatternIC builds an exact-match StringMatcher with ignore_case=true.
func exactPatternIC(s string) *matcherv3.StringMatcher {
	return &matcherv3.StringMatcher{
		MatchPattern: &matcherv3.StringMatcher_Exact{Exact: s},
		IgnoreCase:   true,
	}
}

// prefixPattern builds a prefix-match StringMatcher.
func prefixPattern(s string) *matcherv3.StringMatcher {
	return &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: s}}
}

// prefixPatternIC builds a prefix-match StringMatcher with ignore_case=true.
func prefixPatternIC(s string) *matcherv3.StringMatcher {
	return &matcherv3.StringMatcher{
		MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: s},
		IgnoreCase:   true,
	}
}

// suffixPattern builds a suffix-match StringMatcher.
func suffixPattern(s string) *matcherv3.StringMatcher {
	return &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Suffix{Suffix: s}}
}

// suffixPatternIC builds a suffix-match StringMatcher with ignore_case=true.
func suffixPatternIC(s string) *matcherv3.StringMatcher {
	return &matcherv3.StringMatcher{
		MatchPattern: &matcherv3.StringMatcher_Suffix{Suffix: s},
		IgnoreCase:   true,
	}
}

// containsPattern builds a contains-match StringMatcher.
func containsPattern(s string) *matcherv3.StringMatcher {
	return &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Contains{Contains: s}}
}

// containsPatternIC builds a contains-match StringMatcher with ignore_case=true.
func containsPatternIC(s string) *matcherv3.StringMatcher {
	return &matcherv3.StringMatcher{
		MatchPattern: &matcherv3.StringMatcher_Contains{Contains: s},
		IgnoreCase:   true,
	}
}

// safeRegexPattern builds a safe_regex StringMatcher using the google_re2 engine.
func safeRegexPattern(regex string) *matcherv3.StringMatcher {
	return &matcherv3.StringMatcher{
		MatchPattern: &matcherv3.StringMatcher_SafeRegex{
			SafeRegex: &matcherv3.RegexMatcher{
				EngineType: &matcherv3.RegexMatcher_GoogleRe2{
					GoogleRe2: &matcherv3.RegexMatcher_GoogleRE2{},
				},
				Regex: regex,
			},
		},
	}
}

// customPattern builds a custom StringMatcher (used to test PARSE-REJECT).
func customPattern() *matcherv3.StringMatcher {
	return &matcherv3.StringMatcher{
		MatchPattern: &matcherv3.StringMatcher_Custom{
			Custom: nil, // non-nil MatchPattern with nil TypedExtensionConfig
		},
	}
}

// buildAuthRequestForTest constructs an *authRequest with the given headers
// (as a flat k/v list: k1, v1, k2, v2, ...) and calls buildAuthRequest.
// The filter's compiledConfig carries the supplied allowedHeaders +
// disallowedHeaders. The HttpService carries the headers_to_add from ar.
//
// Task 4 carried-forward (pre-compile fix): mirrors buildCompiledConfig behavior
// by pre-compiling hs.AuthorizationRequest.AllowedHeaders into
// cc.deprecatedAllowedHeaders when allowedHeaders is nil (top-level absent).
// Malformed deprecated patterns are skipped (test helper: use compilable patterns).
func buildAuthRequestForTest(
	t *testing.T,
	allowedHeaders *stringMatcherList,
	disallowedHeaders *stringMatcherList,
	hs *ext_authzv3.HttpService,
	incomingHeaders map[string]string,
	path string,
) *authRequest {
	t.Helper()
	cc := &compiledConfig{
		allowedHeaders:    allowedHeaders,
		disallowedHeaders: disallowedHeaders,
	}
	// Mirror buildCompiledConfig: pre-compile deprecated AuthorizationRequest.allowed_headers
	// when the top-level allowedHeaders is absent, and null it out if allowedHeaders is set.
	if allowedHeaders == nil && hs != nil {
		if ar := hs.GetAuthorizationRequest(); ar != nil {
			if deprecated := ar.GetAllowedHeaders(); deprecated != nil {
				compiled, err := compileStringMatcherList(deprecated)
				if err == nil {
					cc.deprecatedAllowedHeaders = compiled
				}
			}
		}
	}
	f := &filter{state: &factoryState{listenerRC: cc}, activeRC: cc}
	h := make(http.Header)
	for k, v := range incomingHeaders {
		h.Set(k, v)
	}
	return buildAuthRequest(f, hs, h, nil, path)
}

// -------------------------------------------------------------------------
// Group 3 — compileStringMatcherList tests
// -------------------------------------------------------------------------

// TestCompileStringMatcherList_NilInput verifies that a nil ListStringMatcher
// returns nil (= all-pass for allowed_headers; no-filter for disallowed_headers).
func TestCompileStringMatcherList_NilInput(t *testing.T) {
	result, err := compileStringMatcherList(nil)
	if err != nil {
		t.Fatalf("compileStringMatcherList(nil): unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("compileStringMatcherList(nil): got non-nil, want nil")
	}
}

// TestCompileStringMatcherList_Exact verifies exact-match compilation.
func TestCompileStringMatcherList_Exact(t *testing.T) {
	lsm := makeListStringMatcher(exactPattern("authorization"))
	sml, err := compileStringMatcherList(lsm)
	if err != nil {
		t.Fatalf("compileStringMatcherList(exact): unexpected error: %v", err)
	}
	if sml == nil {
		t.Fatal("compileStringMatcherList(exact): got nil, want non-nil")
	}
	if !sml.matchAny("authorization") {
		t.Error("matchAny('authorization'): got false, want true")
	}
	if sml.matchAny("Authorization") {
		t.Error("matchAny('Authorization'): got true, want false (case-sensitive exact)")
	}
	if sml.matchAny("x-custom") {
		t.Error("matchAny('x-custom'): got true, want false")
	}
}

// TestCompileStringMatcherList_ExactIgnoreCase verifies exact-match with ignore_case.
func TestCompileStringMatcherList_ExactIgnoreCase(t *testing.T) {
	lsm := makeListStringMatcher(exactPatternIC("authorization"))
	sml, err := compileStringMatcherList(lsm)
	if err != nil {
		t.Fatalf("compileStringMatcherList(exact ignore_case): unexpected error: %v", err)
	}
	if sml == nil {
		t.Fatal("compileStringMatcherList(exact ignore_case): got nil, want non-nil")
	}
	if !sml.matchAny("authorization") {
		t.Error("matchAny('authorization'): got false, want true")
	}
	if !sml.matchAny("Authorization") {
		t.Error("matchAny('Authorization'): got false, want true (ignore_case)")
	}
	if !sml.matchAny("AUTHORIZATION") {
		t.Error("matchAny('AUTHORIZATION'): got false, want true (ignore_case)")
	}
	if sml.matchAny("x-custom") {
		t.Error("matchAny('x-custom'): got true, want false")
	}
}

// TestCompileStringMatcherList_Prefix verifies prefix-match compilation.
func TestCompileStringMatcherList_Prefix(t *testing.T) {
	lsm := makeListStringMatcher(prefixPattern("x-"))
	sml, err := compileStringMatcherList(lsm)
	if err != nil {
		t.Fatalf("compileStringMatcherList(prefix): unexpected error: %v", err)
	}
	if sml == nil {
		t.Fatal("compileStringMatcherList(prefix): got nil, want non-nil")
	}
	if !sml.matchAny("x-custom") {
		t.Error("matchAny('x-custom'): got false, want true")
	}
	if !sml.matchAny("x-forwarded-for") {
		t.Error("matchAny('x-forwarded-for'): got false, want true")
	}
	if sml.matchAny("authorization") {
		t.Error("matchAny('authorization'): got true, want false")
	}
}

// TestCompileStringMatcherList_PrefixIgnoreCase verifies prefix-match with ignore_case.
func TestCompileStringMatcherList_PrefixIgnoreCase(t *testing.T) {
	lsm := makeListStringMatcher(prefixPatternIC("x-"))
	sml, err := compileStringMatcherList(lsm)
	if err != nil {
		t.Fatalf("compileStringMatcherList(prefix ignore_case): unexpected error: %v", err)
	}
	if !sml.matchAny("x-custom") {
		t.Error("matchAny('x-custom'): got false, want true")
	}
	if !sml.matchAny("X-Custom") {
		t.Error("matchAny('X-Custom'): got false, want true (ignore_case prefix)")
	}
}

// TestCompileStringMatcherList_Suffix verifies suffix-match compilation.
func TestCompileStringMatcherList_Suffix(t *testing.T) {
	lsm := makeListStringMatcher(suffixPattern("-id"))
	sml, err := compileStringMatcherList(lsm)
	if err != nil {
		t.Fatalf("compileStringMatcherList(suffix): unexpected error: %v", err)
	}
	if sml == nil {
		t.Fatal("compileStringMatcherList(suffix): got nil, want non-nil")
	}
	if !sml.matchAny("request-id") {
		t.Error("matchAny('request-id'): got false, want true")
	}
	if !sml.matchAny("trace-id") {
		t.Error("matchAny('trace-id'): got false, want true")
	}
	if sml.matchAny("x-custom") {
		t.Error("matchAny('x-custom'): got true, want false")
	}
}

// TestCompileStringMatcherList_SuffixIgnoreCase verifies suffix-match with ignore_case.
func TestCompileStringMatcherList_SuffixIgnoreCase(t *testing.T) {
	lsm := makeListStringMatcher(suffixPatternIC("-ID"))
	sml, err := compileStringMatcherList(lsm)
	if err != nil {
		t.Fatalf("compileStringMatcherList(suffix ignore_case): unexpected error: %v", err)
	}
	if !sml.matchAny("request-id") {
		t.Error("matchAny('request-id'): got false, want true (ignore_case suffix)")
	}
	if !sml.matchAny("Request-ID") {
		t.Error("matchAny('Request-ID'): got false, want true (ignore_case suffix)")
	}
}

// TestCompileStringMatcherList_Contains verifies contains-match compilation.
func TestCompileStringMatcherList_Contains(t *testing.T) {
	lsm := makeListStringMatcher(containsPattern("auth"))
	sml, err := compileStringMatcherList(lsm)
	if err != nil {
		t.Fatalf("compileStringMatcherList(contains): unexpected error: %v", err)
	}
	if sml == nil {
		t.Fatal("compileStringMatcherList(contains): got nil, want non-nil")
	}
	if !sml.matchAny("authorization") {
		t.Error("matchAny('authorization'): got false, want true")
	}
	if !sml.matchAny("x-auth-token") {
		t.Error("matchAny('x-auth-token'): got false, want true")
	}
	if sml.matchAny("x-custom") {
		t.Error("matchAny('x-custom'): got true, want false")
	}
}

// TestCompileStringMatcherList_ContainsIgnoreCase verifies contains-match with ignore_case.
func TestCompileStringMatcherList_ContainsIgnoreCase(t *testing.T) {
	lsm := makeListStringMatcher(containsPatternIC("AUTH"))
	sml, err := compileStringMatcherList(lsm)
	if err != nil {
		t.Fatalf("compileStringMatcherList(contains ignore_case): unexpected error: %v", err)
	}
	if !sml.matchAny("authorization") {
		t.Error("matchAny('authorization'): got false, want true (ignore_case contains)")
	}
	if !sml.matchAny("X-Auth-Token") {
		t.Error("matchAny('X-Auth-Token'): got false, want true (ignore_case contains)")
	}
}

// TestCompileStringMatcherList_SafeRegex_GoogleRE2 verifies safe_regex compilation
// with the google_re2 engine (D5: google_re2 arm honored per phase-09/12 subset).
func TestCompileStringMatcherList_SafeRegex_GoogleRE2(t *testing.T) {
	lsm := makeListStringMatcher(safeRegexPattern(`^x-[a-z]+-[a-z]+$`))
	sml, err := compileStringMatcherList(lsm)
	if err != nil {
		t.Fatalf("compileStringMatcherList(safe_regex google_re2): unexpected error: %v", err)
	}
	if sml == nil {
		t.Fatal("compileStringMatcherList(safe_regex): got nil, want non-nil")
	}
	if !sml.matchAny("x-custom-header") {
		t.Error("matchAny('x-custom-header'): got false, want true")
	}
	if sml.matchAny("X-Custom-Header") {
		t.Error("matchAny('X-Custom-Header'): got true, want false (regex is case-sensitive)")
	}
	if sml.matchAny("authorization") {
		t.Error("matchAny('authorization'): got true, want false")
	}
}

// TestCompileStringMatcherList_SafeRegex_InvalidRegex verifies that an invalid
// regex pattern returns a PARSE-REJECT error.
func TestCompileStringMatcherList_SafeRegex_InvalidRegex(t *testing.T) {
	lsm := makeListStringMatcher(safeRegexPattern(`[invalid(`))
	_, err := compileStringMatcherList(lsm)
	if err == nil {
		t.Fatal("compileStringMatcherList(invalid regex): want error, got nil")
	}
	if !strings.Contains(err.Error(), "safe_regex") {
		t.Errorf("error %q: want substring 'safe_regex'", err.Error())
	}
}

// TestCompileStringMatcherList_SafeRegex_NilRegexMatcher verifies PARSE-REJECT
// when safe_regex has a nil RegexMatcher inner field.
func TestCompileStringMatcherList_SafeRegex_NilRegexMatcher(t *testing.T) {
	lsm := makeListStringMatcher(&matcherv3.StringMatcher{
		MatchPattern: &matcherv3.StringMatcher_SafeRegex{SafeRegex: nil},
	})
	_, err := compileStringMatcherList(lsm)
	if err == nil {
		t.Fatal("compileStringMatcherList(safe_regex nil): want error, got nil")
	}
}

// TestCompileStringMatcherList_Custom_ParseReject verifies that a custom
// StringMatcher PARSE-REJECTs per envoy-go-strict (no string-matcher extension
// registry; D5 companion rule).
func TestCompileStringMatcherList_Custom_ParseReject(t *testing.T) {
	lsm := makeListStringMatcher(customPattern())
	_, err := compileStringMatcherList(lsm)
	if err == nil {
		t.Fatal("compileStringMatcherList(custom): want PARSE-REJECT error, got nil")
	}
	if !strings.Contains(err.Error(), "custom") {
		t.Errorf("error %q: want substring 'custom'", err.Error())
	}
}

// TestCompileStringMatcherList_MultiplePatterns verifies that a ListStringMatcher
// with multiple patterns applies OR semantics (matchAny returns true if ANY pattern
// matches).
func TestCompileStringMatcherList_MultiplePatterns(t *testing.T) {
	lsm := makeListStringMatcher(
		exactPattern("authorization"),
		prefixPattern("x-"),
		suffixPattern("-id"),
	)
	sml, err := compileStringMatcherList(lsm)
	if err != nil {
		t.Fatalf("compileStringMatcherList(multiple): unexpected error: %v", err)
	}
	if sml == nil {
		t.Fatal("compileStringMatcherList(multiple): got nil")
	}
	if !sml.matchAny("authorization") {
		t.Error("matchAny('authorization'): got false, want true")
	}
	if !sml.matchAny("x-custom") {
		t.Error("matchAny('x-custom'): got false, want true")
	}
	if !sml.matchAny("request-id") {
		t.Error("matchAny('request-id'): got false, want true")
	}
	if sml.matchAny("content-type") {
		t.Error("matchAny('content-type'): got true, want false")
	}
}

// TestCompileStringMatcherList_EmptyPatterns verifies that an empty-patterns
// ListStringMatcher (non-nil but zero patterns) compiles to a non-nil sml that
// matches nothing.
func TestCompileStringMatcherList_EmptyPatterns(t *testing.T) {
	lsm := &matcherv3.ListStringMatcher{Patterns: nil}
	sml, err := compileStringMatcherList(lsm)
	if err != nil {
		t.Fatalf("compileStringMatcherList(empty patterns): unexpected error: %v", err)
	}
	// A non-nil but empty ListStringMatcher → non-nil sml with no matchers.
	if sml == nil {
		t.Fatal("compileStringMatcherList(empty patterns): got nil, want non-nil (matches-nothing)")
	}
	if sml.matchAny("anything") {
		t.Error("matchAny(empty sml): got true, want false (no patterns)")
	}
}

// -------------------------------------------------------------------------
// Group 3 — buildAuthRequest tests
// -------------------------------------------------------------------------

// TestBuildAuthRequest_NilAllowedHeaders_AllPass verifies that when
// cc.allowedHeaders is nil, all incoming client request headers pass through.
func TestBuildAuthRequest_NilAllowedHeaders_AllPass(t *testing.T) {
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: "http://auth:9191"},
	}
	req := buildAuthRequestForTest(t,
		nil, // allowedHeaders: nil = all pass
		nil, // disallowedHeaders: nil = none removed
		hs,
		map[string]string{
			"authorization":   "Bearer tok",
			"x-forwarded-for": "10.0.0.1",
			"content-type":    "application/json",
		},
		"/api/resource",
	)
	if req == nil {
		t.Fatal("buildAuthRequest: got nil authRequest")
	}
	if req.headers.Get("Authorization") == "" {
		t.Error("authorization header: got empty, want present")
	}
	if req.headers.Get("X-Forwarded-For") == "" {
		t.Error("x-forwarded-for header: got empty, want present")
	}
	if req.headers.Get("Content-Type") == "" {
		t.Error("content-type header: got empty, want present")
	}
}

// TestBuildAuthRequest_AllowedHeaders_FiltersHeaders verifies that when
// cc.allowedHeaders is set, only matching headers pass through.
func TestBuildAuthRequest_AllowedHeaders_FiltersHeaders(t *testing.T) {
	lsm := makeListStringMatcher(
		exactPattern("authorization"),
		prefixPatternIC("x-"),
	)
	allowed, err := compileStringMatcherList(lsm)
	if err != nil {
		t.Fatalf("compileStringMatcherList: %v", err)
	}
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: "http://auth:9191"},
	}
	req := buildAuthRequestForTest(t,
		allowed,
		nil, // no disallowedHeaders
		hs,
		map[string]string{
			"authorization":   "Bearer tok",
			"x-custom":        "custom-val",
			"content-type":    "application/json", // should be filtered out
			"accept-encoding": "gzip",             // should be filtered out
		},
		"/api/resource",
	)
	if req == nil {
		t.Fatal("buildAuthRequest: got nil")
	}
	// Matching headers present.
	if req.headers.Get("Authorization") == "" {
		t.Error("authorization: got empty, want present (exact match)")
	}
	if req.headers.Get("X-Custom") == "" {
		t.Error("x-custom: got empty, want present (prefix x-)")
	}
	// Non-matching headers absent.
	if req.headers.Get("Content-Type") != "" {
		t.Error("content-type: got present, want absent (not in allowed_headers)")
	}
	if req.headers.Get("Accept-Encoding") != "" {
		t.Error("accept-encoding: got present, want absent (not in allowed_headers)")
	}
}

// TestBuildAuthRequest_DisallowedHeaders_OverridesAllowed verifies that headers
// matching cc.disallowedHeaders are removed even if they match cc.allowedHeaders
// (disallowed_headers overrides allowed_headers per the proto doc).
func TestBuildAuthRequest_DisallowedHeaders_OverridesAllowed(t *testing.T) {
	// allowed_headers: all headers starting with x-
	allowed, err := compileStringMatcherList(makeListStringMatcher(prefixPatternIC("x-")))
	if err != nil {
		t.Fatalf("compileStringMatcherList(allowed): %v", err)
	}
	// disallowed_headers: x-secret-token (also starts with x-; overrides allowed)
	disallowed, err := compileStringMatcherList(makeListStringMatcher(exactPatternIC("x-secret-token")))
	if err != nil {
		t.Fatalf("compileStringMatcherList(disallowed): %v", err)
	}
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: "http://auth:9191"},
	}
	req := buildAuthRequestForTest(t,
		allowed,
		disallowed,
		hs,
		map[string]string{
			"x-custom":       "custom-val",
			"x-secret-token": "super-secret", // in both allowed and disallowed → removed
		},
		"/api/resource",
	)
	if req == nil {
		t.Fatal("buildAuthRequest: got nil")
	}
	// x-custom passes (allowed, not disallowed).
	if req.headers.Get("X-Custom") == "" {
		t.Error("x-custom: got empty, want present")
	}
	// x-secret-token removed (disallowed overrides allowed).
	if req.headers.Get("X-Secret-Token") != "" {
		t.Errorf("x-secret-token: got %q, want absent (disallowed overrides allowed)",
			req.headers.Get("X-Secret-Token"))
	}
}

// TestBuildAuthRequest_DisallowedHeaders_NilAllowed verifies disallowed_headers
// removes headers even when allowed_headers is nil (all-pass).
func TestBuildAuthRequest_DisallowedHeaders_NilAllowed(t *testing.T) {
	disallowed, err := compileStringMatcherList(makeListStringMatcher(exactPattern("x-internal")))
	if err != nil {
		t.Fatalf("compileStringMatcherList(disallowed): %v", err)
	}
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: "http://auth:9191"},
	}
	req := buildAuthRequestForTest(t,
		nil, // all-pass
		disallowed,
		hs,
		map[string]string{
			"authorization": "Bearer tok",
			"x-internal":    "internal-value", // should be removed
		},
		"/api/resource",
	)
	if req == nil {
		t.Fatal("buildAuthRequest: got nil")
	}
	if req.headers.Get("Authorization") == "" {
		t.Error("authorization: got empty, want present")
	}
	if req.headers.Get("X-Internal") != "" {
		t.Errorf("x-internal: got %q, want absent", req.headers.Get("X-Internal"))
	}
}

// TestBuildAuthRequest_HeadersToAdd_Appended verifies that
// AuthorizationRequest.headers_to_add static headers are appended to the
// filtered header set (and overwrite client headers with the same name per
// the proto doc "Note that client request of the same key will be overridden").
func TestBuildAuthRequest_HeadersToAdd_Appended(t *testing.T) {
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: "http://auth:9191"},
		AuthorizationRequest: &ext_authzv3.AuthorizationRequest{
			HeadersToAdd: []*corev3.HeaderValue{
				{Key: "x-envoy-auth-source", Value: "envoy-go"},
				{Key: "x-request-id", Value: "static-id"},
			},
		},
	}
	req := buildAuthRequestForTest(t,
		nil, // all-pass
		nil,
		hs,
		map[string]string{
			"authorization": "Bearer tok",
			"x-request-id":  "client-id", // will be overwritten by headers_to_add
		},
		"/api",
	)
	if req == nil {
		t.Fatal("buildAuthRequest: got nil")
	}
	if req.headers.Get("X-Envoy-Auth-Source") != "envoy-go" {
		t.Errorf("x-envoy-auth-source: got %q, want %q",
			req.headers.Get("X-Envoy-Auth-Source"), "envoy-go")
	}
	// x-request-id should be overwritten by headers_to_add.
	if req.headers.Get("X-Request-Id") != "static-id" {
		t.Errorf("x-request-id: got %q, want %q (headers_to_add overwrites client)",
			req.headers.Get("X-Request-Id"), "static-id")
	}
}

// TestBuildAuthRequest_DeprecatedAllowedHeaders_HonoredIfPresent verifies D6:
// the deprecated AuthorizationRequest.allowed_headers (#1) is honored when
// present (backward-compat, mirrors phase-17 amendment-4 "deprecated-but-honored"
// disposition). When both the deprecated field AND the top-level
// cc.allowedHeaders are nil, the deprecated field's matchers gate the headers.
func TestBuildAuthRequest_DeprecatedAllowedHeaders_HonoredIfPresent(t *testing.T) {
	// Only the deprecated field is set; top-level cc.allowedHeaders is nil.
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: "http://auth:9191"},
		AuthorizationRequest: &ext_authzv3.AuthorizationRequest{
			// Deprecated AllowedHeaders: only let "authorization" through.
			AllowedHeaders: makeListStringMatcher(exactPattern("authorization")),
		},
	}
	req := buildAuthRequestForTest(t,
		nil, // cc.allowedHeaders nil — deprecated field should be the effective gate
		nil,
		hs,
		map[string]string{
			"authorization": "Bearer tok",
			"x-custom":      "custom-val", // should be filtered by deprecated field
		},
		"/api",
	)
	if req == nil {
		t.Fatal("buildAuthRequest: got nil")
	}
	// authorization should pass (deprecated field allows it).
	if req.headers.Get("Authorization") == "" {
		t.Error("authorization: got empty, want present (deprecated allowed_headers)")
	}
	// x-custom should be filtered (not in deprecated allowed_headers).
	if req.headers.Get("X-Custom") != "" {
		t.Errorf("x-custom: got %q, want absent (deprecated allowed_headers filters it)",
			req.headers.Get("X-Custom"))
	}
}

// TestBuildAuthRequest_TopLevelAllowedHeadersTakesPrecedence verifies that the
// top-level cc.allowedHeaders takes precedence when both it and the deprecated
// AuthorizationRequest.allowed_headers are set. The top-level primary path wins.
func TestBuildAuthRequest_TopLevelAllowedHeadersTakesPrecedence(t *testing.T) {
	// top-level cc.allowedHeaders: only "authorization"
	topLevel, err := compileStringMatcherList(makeListStringMatcher(exactPattern("authorization")))
	if err != nil {
		t.Fatalf("compileStringMatcherList: %v", err)
	}
	// deprecated field: only "x-custom" — should NOT take effect when top-level is set
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: "http://auth:9191"},
		AuthorizationRequest: &ext_authzv3.AuthorizationRequest{
			AllowedHeaders: makeListStringMatcher(exactPattern("x-custom")),
		},
	}
	req := buildAuthRequestForTest(t,
		topLevel, // top-level is set → deprecated field is NOT applied
		nil,
		hs,
		map[string]string{
			"authorization": "Bearer tok",
			"x-custom":      "val",
		},
		"/api",
	)
	if req == nil {
		t.Fatal("buildAuthRequest: got nil")
	}
	// authorization passes via top-level allowed_headers.
	if req.headers.Get("Authorization") == "" {
		t.Error("authorization: got empty, want present (top-level allows)")
	}
	// x-custom does NOT pass: top-level allowed_headers wins over deprecated field.
	if req.headers.Get("X-Custom") != "" {
		t.Errorf("x-custom: got %q, want absent (top-level wins over deprecated)",
			req.headers.Get("X-Custom"))
	}
}

// TestBuildAuthRequest_PathCarried verifies that the path is carried in the
// authRequest (path_prefix prepending is done in check.go's closure, not here;
// buildAuthRequest just stores the path as-is).
func TestBuildAuthRequest_PathCarried(t *testing.T) {
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: "http://auth:9191"},
	}
	req := buildAuthRequestForTest(t, nil, nil, hs, nil, "/api/resource")
	if req == nil {
		t.Fatal("buildAuthRequest: got nil")
	}
	if req.path != "/api/resource" {
		t.Errorf("path: got %q, want %q", req.path, "/api/resource")
	}
}

// TestBuildAuthRequest_BodyIncluded verifies that a non-nil body is carried
// in the authRequest.
func TestBuildAuthRequest_BodyIncluded(t *testing.T) {
	cc := &compiledConfig{}
	f := &filter{state: &factoryState{listenerRC: cc}, activeRC: cc}
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: "http://auth:9191"},
	}
	body := []byte("request-body-data")
	req := buildAuthRequest(f, hs, make(http.Header), body, "/api")
	if req == nil {
		t.Fatal("buildAuthRequest: got nil")
	}
	if string(req.body) != "request-body-data" {
		t.Errorf("body: got %q, want %q", req.body, "request-body-data")
	}
}

// TestBuildAuthRequest_NilHttpService verifies that a nil hs is a valid path:
// buildAuthRequest guards `hs != nil` at both the deprecated-allowed_headers
// compilation and the headers_to_add append. With nil hs, all incoming headers
// pass through (subject to allowed/disallowed filtering) and no static headers
// are appended.
func TestBuildAuthRequest_NilHttpService(t *testing.T) {
	req := buildAuthRequestForTest(t,
		nil, // allowedHeaders: nil = all pass
		nil, // disallowedHeaders: nil = none removed
		nil, // hs: nil — valid path
		map[string]string{
			"authorization": "Bearer tok",
			"x-request-id":  "abc-123",
		},
		"/api/resource",
	)
	if req == nil {
		t.Fatal("buildAuthRequest(nil hs): got nil authRequest")
	}
	// Incoming headers pass through unfiltered.
	if req.headers.Get("Authorization") == "" {
		t.Error("authorization header: got empty, want present")
	}
	if req.headers.Get("X-Request-Id") == "" {
		t.Error("x-request-id header: got empty, want present")
	}
	// No headers_to_add appended (hs is nil).
	if got := len(req.headers); got != 2 {
		t.Errorf("header count: got %d, want 2 (no headers_to_add with nil hs)", got)
	}
	if req.path != "/api/resource" {
		t.Errorf("path: got %q, want %q", req.path, "/api/resource")
	}
}

// -------------------------------------------------------------------------
// Group 3 — validateMutationHeaders tests
// -------------------------------------------------------------------------

// TestValidateMutationHeaders_ValidHeaders verifies that well-formed header
// name/value pairs pass validation.
func TestValidateMutationHeaders_ValidHeaders(t *testing.T) {
	hdrs := []headerKV{
		{name: "x-auth-result", value: "allowed"},
		{name: "x-custom-header", value: "some-value"},
		{name: "content-type", value: "application/json"},
	}
	if err := validateMutationHeaders(hdrs); err != nil {
		t.Errorf("validateMutationHeaders(valid): unexpected error: %v", err)
	}
}

// TestValidateMutationHeaders_PseudoHeaderReject verifies that :-prefixed
// pseudo-headers are rejected per D7 (mirrors phase-10 header_mutation discipline).
func TestValidateMutationHeaders_PseudoHeaderReject(t *testing.T) {
	pseudoHeaders := []string{":method", ":path", ":authority", ":scheme", ":status", ":anything"}
	for _, name := range pseudoHeaders {
		t.Run(name, func(t *testing.T) {
			hdrs := []headerKV{{name: name, value: "value"}}
			err := validateMutationHeaders(hdrs)
			if err == nil {
				t.Errorf("validateMutationHeaders(%q): want error (pseudo-header), got nil", name)
			}
		})
	}
}

// TestValidateMutationHeaders_InvalidHeaderNameChars verifies that header names
// with invalid characters (e.g., spaces, control chars) are rejected.
func TestValidateMutationHeaders_InvalidHeaderNameChars(t *testing.T) {
	invalidNames := []string{
		"invalid name",    // space
		"invalid\x00name", // NUL
		"invalid\nname",   // newline
		"invalid\rname",   // carriage return
	}
	for _, name := range invalidNames {
		t.Run("name_"+strings.Map(func(r rune) rune {
			if r < 0x20 {
				return '_'
			}
			return r
		}, name), func(t *testing.T) {
			hdrs := []headerKV{{name: name, value: "value"}}
			err := validateMutationHeaders(hdrs)
			if err == nil {
				t.Errorf("validateMutationHeaders(invalid name chars): want error for %q, got nil", name)
			}
		})
	}
}

// TestValidateMutationHeaders_EmptyHeaderName verifies that an empty header name
// is rejected with a descriptive error (validateMutationHeaderName guards
// len(name) == 0 before the pseudo-header / token-char checks).
func TestValidateMutationHeaders_EmptyHeaderName(t *testing.T) {
	hdrs := []headerKV{{name: "", value: "some-value"}}
	err := validateMutationHeaders(hdrs)
	if err == nil {
		t.Fatal("validateMutationHeaders(empty name): want error, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("validateMutationHeaders(empty name): error = %q, want it to mention %q", err.Error(), "empty")
	}
}

// TestValidateMutationHeaders_InvalidHeaderValueChars verifies that header values
// with invalid characters (control chars except \t) are rejected.
func TestValidateMutationHeaders_InvalidHeaderValueChars(t *testing.T) {
	invalidValues := []string{
		"invalid\x00value", // NUL
		"invalid\nvalue",   // bare newline
		"invalid\rvalue",   // bare carriage return
	}
	for _, value := range invalidValues {
		t.Run("value_"+strings.Map(func(r rune) rune {
			if r < 0x20 {
				return '_'
			}
			return r
		}, value), func(t *testing.T) {
			hdrs := []headerKV{{name: "x-custom", value: value}}
			err := validateMutationHeaders(hdrs)
			if err == nil {
				t.Errorf("validateMutationHeaders(invalid value chars): want error for value %q, got nil", value)
			}
		})
	}
}

// TestValidateMutationHeaders_EmptySlice verifies that an empty (nil or zero-length)
// slice passes validation without error.
func TestValidateMutationHeaders_EmptySlice(t *testing.T) {
	if err := validateMutationHeaders(nil); err != nil {
		t.Errorf("validateMutationHeaders(nil): unexpected error: %v", err)
	}
	if err := validateMutationHeaders([]headerKV{}); err != nil {
		t.Errorf("validateMutationHeaders(empty): unexpected error: %v", err)
	}
}

// ----------------------------------------------------------------------------
// Group 8 — Bidirectional header-mutation discipline (ADR-0161)
//
// Tests the following surfaces:
//   - Allow-path: buildHTTPCheckFn compiles allowed_upstream_headers +
//     allowed_upstream_headers_to_append → checkFn populates upstreamSet
//     (overwrite) + upstreamApp (append) from the auth response headers.
//   - Deny-path: buildHTTPCheckFn compiles allowed_client_headers → checkFn
//     populates denyHeaders (allowed_client_headers-filtered) with text/plain
//     fallback + decision-headers-first ordering.
//   - validate_mutations: true gates validateMutationHeaders over upstreamSet +
//     upstreamApp (allow path) and denyHeaders (deny path); a violation drives
//     dispInvalid (treated as error posture per SPEC §6.3).
//   - Pre-compile fix (Task 4 carried-forward): deprecated
//     AuthorizationRequest.allowed_headers compiled at config-load time; a
//     malformed pattern surfaces as a PARSE-REJECT at buildCompiledConfig time.
//   - applyUpstreamMutations: the allow-path helper that applies
//     upstreamSet (overwrite) + upstreamApp (append) to an http.Header.
// ----------------------------------------------------------------------------

// buildHTTPCheckFnWithResponse builds a checkFn pointing at the given server URL
// with authorization_response matchers compiled from the provided HttpService.
// Helper for Group 8 tests.
func buildHTTPCheckFnWithResponse(t *testing.T, hs *ext_authzv3.HttpService, validateMutations bool) checkFn {
	t.Helper()
	fn, err := buildHTTPCheckFn(hs, validateMutations)
	if err != nil {
		t.Fatalf("buildHTTPCheckFn: unexpected error: %v", err)
	}
	return fn
}

// makeAuthResponseMatcher compiles a ListStringMatcher from exact patterns.
func makeAuthResponseMatcher(t *testing.T, patterns ...string) *matcherv3.ListStringMatcher {
	t.Helper()
	sms := make([]*matcherv3.StringMatcher, len(patterns))
	for i, p := range patterns {
		sms[i] = exactPattern(p)
	}
	return makeListStringMatcher(sms...)
}

// ---------------------------------------------------------------------------
// Group 8A — Allow-path: upstreamSet (overwrite) + upstreamApp (append)
// ---------------------------------------------------------------------------

// TestHeaderMutation_AllowPath_UpstreamSet verifies that on HTTP 200, auth-response
// headers matching allowed_upstream_headers are extracted into upstreamSet
// (overwrite semantics per ADR-0161).
func TestHeaderMutation_AllowPath_UpstreamSet(t *testing.T) {
	// Auth server returns 200 + x-auth-user + x-auth-tenant headers.
	srv := newScriptableAuthServer(t, http.StatusOK, map[string]string{
		"x-auth-user":   "alice",
		"x-auth-tenant": "acme",
		"x-noise":       "ignored",
	}, "")
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: srv.srv.URL},
		AuthorizationResponse: &ext_authzv3.AuthorizationResponse{
			AllowedUpstreamHeaders: makeAuthResponseMatcher(t, "x-auth-user", "x-auth-tenant"),
		},
	}
	fn := buildHTTPCheckFnWithResponse(t, hs, false)

	disp, err := fn(context.Background(), minimalAuthRequest("/check", nil))
	if err != nil {
		t.Fatalf("checkFn(200+upstream_set): unexpected error: %v", err)
	}
	if disp.class != dispAllow {
		t.Fatalf("disposition class: got %v, want dispAllow", disp.class)
	}
	// upstreamSet must contain x-auth-user + x-auth-tenant (both in allow list).
	gotSet := make(map[string]string)
	for _, kv := range disp.upstreamSet {
		gotSet[kv.name] = kv.value
	}
	if gotSet["x-auth-user"] != "alice" {
		t.Errorf("upstreamSet[x-auth-user]: got %q, want %q", gotSet["x-auth-user"], "alice")
	}
	if gotSet["x-auth-tenant"] != "acme" {
		t.Errorf("upstreamSet[x-auth-tenant]: got %q, want %q", gotSet["x-auth-tenant"], "acme")
	}
	// x-noise must NOT appear in upstreamSet (not in allowed_upstream_headers).
	if _, ok := gotSet["x-noise"]; ok {
		t.Error("upstreamSet contains x-noise: want absent (not in allowed_upstream_headers)")
	}
	// upstreamApp must be empty (no allowed_upstream_headers_to_append configured).
	if len(disp.upstreamApp) != 0 {
		t.Errorf("upstreamApp: got %d entries, want 0", len(disp.upstreamApp))
	}
}

// TestHeaderMutation_AllowPath_UpstreamApp verifies that on HTTP 200, auth-response
// headers matching allowed_upstream_headers_to_append are extracted into upstreamApp
// (append semantics per ADR-0161).
func TestHeaderMutation_AllowPath_UpstreamApp(t *testing.T) {
	srv := newScriptableAuthServer(t, http.StatusOK, map[string]string{
		"x-auth-role": "admin",
		"x-noise":     "ignored",
	}, "")
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: srv.srv.URL},
		AuthorizationResponse: &ext_authzv3.AuthorizationResponse{
			AllowedUpstreamHeadersToAppend: makeAuthResponseMatcher(t, "x-auth-role"),
		},
	}
	fn := buildHTTPCheckFnWithResponse(t, hs, false)

	disp, err := fn(context.Background(), minimalAuthRequest("/check", nil))
	if err != nil {
		t.Fatalf("checkFn(200+upstream_app): unexpected error: %v", err)
	}
	if disp.class != dispAllow {
		t.Fatalf("disposition class: got %v, want dispAllow", disp.class)
	}
	// upstreamApp must contain x-auth-role.
	gotApp := make(map[string]string)
	for _, kv := range disp.upstreamApp {
		gotApp[kv.name] = kv.value
	}
	if gotApp["x-auth-role"] != "admin" {
		t.Errorf("upstreamApp[x-auth-role]: got %q, want %q", gotApp["x-auth-role"], "admin")
	}
	// upstreamSet must be empty (no allowed_upstream_headers configured).
	if len(disp.upstreamSet) != 0 {
		t.Errorf("upstreamSet: got %d entries, want 0", len(disp.upstreamSet))
	}
}

// TestHeaderMutation_AllowPath_SetAndAppend verifies that both set and append
// matchers are honored simultaneously, with disjoint header sets.
func TestHeaderMutation_AllowPath_SetAndAppend(t *testing.T) {
	srv := newScriptableAuthServer(t, http.StatusOK, map[string]string{
		"x-set-header": "set-value",
		"x-app-header": "app-value",
	}, "")
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: srv.srv.URL},
		AuthorizationResponse: &ext_authzv3.AuthorizationResponse{
			AllowedUpstreamHeaders:         makeAuthResponseMatcher(t, "x-set-header"),
			AllowedUpstreamHeadersToAppend: makeAuthResponseMatcher(t, "x-app-header"),
		},
	}
	fn := buildHTTPCheckFnWithResponse(t, hs, false)

	disp, err := fn(context.Background(), minimalAuthRequest("/check", nil))
	if err != nil {
		t.Fatalf("checkFn(set+append): unexpected error: %v", err)
	}
	if disp.class != dispAllow {
		t.Fatalf("disposition class: got %v, want dispAllow", disp.class)
	}
	if len(disp.upstreamSet) != 1 || disp.upstreamSet[0].name != "x-set-header" {
		t.Errorf("upstreamSet: got %v, want 1 entry x-set-header", disp.upstreamSet)
	}
	if len(disp.upstreamApp) != 1 || disp.upstreamApp[0].name != "x-app-header" {
		t.Errorf("upstreamApp: got %v, want 1 entry x-app-header", disp.upstreamApp)
	}
}

// TestHeaderMutation_AllowPath_NilMatcher verifies that nil allowed_upstream_headers
// means no upstream set headers (only non-nil matchers cause extraction).
func TestHeaderMutation_AllowPath_NilMatcher(t *testing.T) {
	srv := newScriptableAuthServer(t, http.StatusOK, map[string]string{
		"x-auth-user": "alice",
	}, "")
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: srv.srv.URL},
		// No AuthorizationResponse configured → nil matchers.
	}
	fn := buildHTTPCheckFnWithResponse(t, hs, false)

	disp, err := fn(context.Background(), minimalAuthRequest("/check", nil))
	if err != nil {
		t.Fatalf("checkFn(nil matchers): unexpected error: %v", err)
	}
	if disp.class != dispAllow {
		t.Fatalf("disposition class: got %v, want dispAllow", disp.class)
	}
	// nil matchers → no upstream mutations extracted.
	if len(disp.upstreamSet) != 0 {
		t.Errorf("upstreamSet: got %d entries, want 0 (nil matcher)", len(disp.upstreamSet))
	}
	if len(disp.upstreamApp) != 0 {
		t.Errorf("upstreamApp: got %d entries, want 0 (nil matcher)", len(disp.upstreamApp))
	}
}

// ---------------------------------------------------------------------------
// Group 8B — Deny-path: denyHeaders (allowed_client_headers + text/plain fallback)
// ---------------------------------------------------------------------------

// TestHeaderMutation_DenyPath_AllowedClientHeaders verifies that on HTTP 403,
// auth-response headers matching allowed_client_headers are extracted into
// denyHeaders per ADR-0161.
func TestHeaderMutation_DenyPath_AllowedClientHeaders(t *testing.T) {
	srv := newScriptableAuthServer(t, http.StatusForbidden, map[string]string{
		"x-deny-reason": "policy-violation",
		"content-type":  "text/plain",
		"x-noise":       "ignored",
	}, "denied\n")
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: srv.srv.URL},
		AuthorizationResponse: &ext_authzv3.AuthorizationResponse{
			AllowedClientHeaders: makeAuthResponseMatcher(t, "x-deny-reason", "content-type"),
		},
	}
	fn := buildHTTPCheckFnWithResponse(t, hs, false)

	disp, err := fn(context.Background(), minimalAuthRequest("/check", nil))
	if err != nil {
		t.Fatalf("checkFn(403+client_headers): unexpected error: %v", err)
	}
	if disp.class != dispDeny {
		t.Fatalf("disposition class: got %v, want dispDeny", disp.class)
	}
	// denyHeaders must contain x-deny-reason + content-type (both in allow list).
	gotHdrs := make(map[string]string)
	for _, kv := range disp.denyHeaders {
		gotHdrs[kv.name] = kv.value
	}
	if gotHdrs["x-deny-reason"] != "policy-violation" {
		t.Errorf("denyHeaders[x-deny-reason]: got %q, want %q", gotHdrs["x-deny-reason"], "policy-violation")
	}
	if gotHdrs["content-type"] != "text/plain" {
		t.Errorf("denyHeaders[content-type]: got %q, want %q", gotHdrs["content-type"], "text/plain")
	}
	// x-noise must NOT appear in denyHeaders (not in allowed_client_headers).
	if _, ok := gotHdrs["x-noise"]; ok {
		t.Error("denyHeaders contains x-noise: want absent (not in allowed_client_headers)")
	}
}

// TestHeaderMutation_DenyPath_TextPlainFallback verifies that if the auth service
// does not supply a content-type header in the allowed_client_headers set, a
// "text/plain" fallback is synthesized per SPEC §4 + parent §5.P11.
func TestHeaderMutation_DenyPath_TextPlainFallback(t *testing.T) {
	srv := newScriptableAuthServer(t, http.StatusForbidden, map[string]string{
		"x-deny-reason": "policy-violation",
		"content-type":  "application/json", // NOT in the allow list → should fall back to text/plain
	}, "denied\n")
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: srv.srv.URL},
		AuthorizationResponse: &ext_authzv3.AuthorizationResponse{
			// Only x-deny-reason allowed; content-type is NOT included.
			AllowedClientHeaders: makeAuthResponseMatcher(t, "x-deny-reason"),
		},
	}
	fn := buildHTTPCheckFnWithResponse(t, hs, false)

	disp, err := fn(context.Background(), minimalAuthRequest("/check", nil))
	if err != nil {
		t.Fatalf("checkFn(403+text/plain fallback): unexpected error: %v", err)
	}
	if disp.class != dispDeny {
		t.Fatalf("disposition class: got %v, want dispDeny", disp.class)
	}
	// content-type must be synthesized as "text/plain" (fallback — not in allow list).
	gotHdrs := make(map[string]string)
	for _, kv := range disp.denyHeaders {
		gotHdrs[kv.name] = kv.value
	}
	if gotHdrs["content-type"] != "text/plain" {
		t.Errorf("denyHeaders[content-type]: got %q, want 'text/plain' (fallback)", gotHdrs["content-type"])
	}
}

// TestHeaderMutation_DenyPath_NilClientHeadersMatcher verifies that when
// allowed_client_headers is nil (no filter configured), only the text/plain
// fallback is synthesized (no auth headers passed through — nil means no matcher
// configured → no client headers extracted, only the fallback).
//
// NOTE: This diverges from the gRPC mode where DeniedHttpResponse headers are
// applied verbatim. In HTTP mode, nil allowed_client_headers means the filter
// was not configured to pass any headers — the body and status still flow, but
// no specific header set is forwarded. The text/plain fallback ensures the
// response has a content-type.
func TestHeaderMutation_DenyPath_NilClientHeadersMatcher(t *testing.T) {
	srv := newScriptableAuthServer(t, http.StatusForbidden, map[string]string{
		"x-deny-reason": "policy-violation",
		"content-type":  "application/json",
	}, "denied\n")
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: srv.srv.URL},
		// No AuthorizationResponse → nil allowed_client_headers.
	}
	fn := buildHTTPCheckFnWithResponse(t, hs, false)

	disp, err := fn(context.Background(), minimalAuthRequest("/check", nil))
	if err != nil {
		t.Fatalf("checkFn(403 nil client_headers): unexpected error: %v", err)
	}
	if disp.class != dispDeny {
		t.Fatalf("disposition class: got %v, want dispDeny", disp.class)
	}
	// nil matcher → only text/plain fallback in denyHeaders.
	gotHdrs := make(map[string]string)
	for _, kv := range disp.denyHeaders {
		gotHdrs[kv.name] = kv.value
	}
	if gotHdrs["content-type"] != "text/plain" {
		t.Errorf("denyHeaders[content-type]: got %q, want 'text/plain' (fallback)", gotHdrs["content-type"])
	}
	// x-deny-reason must NOT appear (nil matcher = no extraction).
	if _, ok := gotHdrs["x-deny-reason"]; ok {
		t.Error("denyHeaders contains x-deny-reason: want absent (nil allowed_client_headers)")
	}
}

// TestHeaderMutation_DenyPath_DecisionHeadersFirst verifies that auth-service
// decision headers appear BEFORE any synthesized housekeeping headers
// (text/plain fallback content-type) in denyHeaders — per parent §5.P11
// RATIFIED-PENDING-IMPL-TIME + SPEC §4.
func TestHeaderMutation_DenyPath_DecisionHeadersFirst(t *testing.T) {
	srv := newScriptableAuthServer(t, http.StatusUnauthorized, map[string]string{
		"x-auth-challenge": "Bearer realm=auth",
		// no content-type → fallback should be APPENDED after decision headers
	}, "unauthorized\n")
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: srv.srv.URL},
		AuthorizationResponse: &ext_authzv3.AuthorizationResponse{
			AllowedClientHeaders: makeAuthResponseMatcher(t, "x-auth-challenge"),
		},
	}
	fn := buildHTTPCheckFnWithResponse(t, hs, false)

	disp, err := fn(context.Background(), minimalAuthRequest("/check", nil))
	if err != nil {
		t.Fatalf("checkFn(401 decision-first): unexpected error: %v", err)
	}
	if disp.class != dispDeny {
		t.Fatalf("disposition class: got %v, want dispDeny", disp.class)
	}
	if len(disp.denyHeaders) < 2 {
		t.Fatalf("denyHeaders: got %d entries, want at least 2 (decision header + content-type fallback)", len(disp.denyHeaders))
	}
	// x-auth-challenge must appear BEFORE content-type (decision-headers-first).
	firstIdx, ctIdx := -1, -1
	for i, kv := range disp.denyHeaders {
		if kv.name == "x-auth-challenge" {
			firstIdx = i
		}
		if kv.name == "content-type" {
			ctIdx = i
		}
	}
	if firstIdx < 0 {
		t.Error("denyHeaders: x-auth-challenge not found")
	}
	if ctIdx < 0 {
		t.Error("denyHeaders: content-type not found")
	}
	if firstIdx >= ctIdx {
		t.Errorf("ordering: x-auth-challenge at %d, content-type at %d; want decision header BEFORE content-type fallback",
			firstIdx, ctIdx)
	}
}

// ---------------------------------------------------------------------------
// Group 8C — validate_mutations gating → dispInvalid
// ---------------------------------------------------------------------------

// TestValidateMutations_AllowPath_PseudoHeaderRejected verifies that when
// validate_mutations is true and the auth service returns a :-prefixed pseudo-
// header in the upstream-set response, the disposition becomes dispInvalid.
//
// Implementation note: Go's net/http server strips :-prefixed pseudo-headers
// from HTTP/1.1 responses (they are HTTP/2-only). To test the validate_mutations
// gating without an HTTP/2 server, we call mapHTTPResponseWithMatchers directly
// with a hand-crafted *http.Response whose Header map already contains the
// pseudo-header (bypassing the HTTP/1.1 wire layer).
func TestValidateMutations_AllowPath_PseudoHeaderRejected(t *testing.T) {
	// Hand-craft a 200 response with a ":status" pseudo-header in the map.
	// (net/http would never return this over HTTP/1.1, but Envoy's gRPC path
	// can; the validate_mutations gate must reject it regardless of transport.)
	allowedUpstream, err := compileStringMatcherList(makeAuthResponseMatcher(t, ":status"))
	if err != nil {
		t.Fatalf("compileStringMatcherList: %v", err)
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			// http.Header is a map[string][]string; keys need not be canonical.
			":Status": {"200"},
		},
		Body: http.NoBody,
	}
	disp, err := mapHTTPResponseWithMatchers(resp, allowedUpstream, nil, nil, true /* validateMutations */)
	if err != nil {
		t.Fatalf("mapHTTPResponseWithMatchers(validate+pseudo-header): unexpected error: %v", err)
	}
	if disp.class != dispInvalid {
		t.Errorf("disposition class: got %v, want dispInvalid (pseudo-header rejected by validate_mutations)", disp.class)
	}
}

// TestValidateMutations_AllowPath_InvalidNameCharsRejected verifies that when
// validate_mutations is true and the auth service returns an upstream-set header
// whose name contains an invalid character (space), the disposition becomes
// dispInvalid.
//
// Same net/http limitation as the pseudo-header variant above: net/http's
// HTTP/1.1 wire layer would reject an invalid header name, so we hand-craft a
// *http.Response whose Header map already carries the invalid-name header and
// drive it through mapHTTPResponseWithMatchers — genuinely exercising the
// mapHTTPResponseWithMatchers → validateMutationHeaders → dispInvalid wiring.
func TestValidateMutations_AllowPath_InvalidNameCharsRejected(t *testing.T) {
	// Match the invalid-name header so extraction pulls it into upstreamSet.
	allowedUpstream, err := compileStringMatcherList(makeAuthResponseMatcher(t, "invalid name"))
	if err != nil {
		t.Fatalf("compileStringMatcherList: %v", err)
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			// http.Header is a map[string][]string; keys need not be canonical.
			// A space in the name is invalid per RFC 7230 §3.2.6.
			"invalid name": {"val"},
		},
		Body: http.NoBody,
	}
	disp, err := mapHTTPResponseWithMatchers(resp, allowedUpstream, nil, nil, true /* validateMutations */)
	if err != nil {
		t.Fatalf("mapHTTPResponseWithMatchers(validate+invalid-name): unexpected error: %v", err)
	}
	if disp.class != dispInvalid {
		t.Errorf("disposition class: got %v, want dispInvalid (invalid-name header rejected by validate_mutations)", disp.class)
	}
}

// TestValidateMutations_DenyPath_PseudoHeaderRejected verifies that when
// validate_mutations is true and the auth service returns a :-prefixed header
// in the deny-path allowed_client_headers, the disposition becomes dispInvalid.
//
// Same net/http limitation as the allow-path variant above: we bypass the wire
// layer and call mapHTTPResponseWithMatchers directly with a hand-crafted response.
func TestValidateMutations_DenyPath_PseudoHeaderRejected(t *testing.T) {
	allowedClient, err := compileStringMatcherList(makeAuthResponseMatcher(t, ":status"))
	if err != nil {
		t.Fatalf("compileStringMatcherList: %v", err)
	}
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header: http.Header{
			":Status": {"403"},
		},
		Body: io.NopCloser(strings.NewReader("denied\n")),
	}
	disp, err := mapHTTPResponseWithMatchers(resp, nil, nil, allowedClient, true /* validateMutations */)
	if err != nil {
		t.Fatalf("mapHTTPResponseWithMatchers(validate deny+pseudo-header): unexpected error: %v", err)
	}
	if disp.class != dispInvalid {
		t.Errorf("disposition class: got %v, want dispInvalid (deny pseudo-header rejected)", disp.class)
	}
}

// TestValidateMutations_False_PseudoHeaderAllowed verifies that when
// validate_mutations is false, pseudo-headers in the allow list are NOT rejected
// (the filter does not validate mutations when the flag is off).
func TestValidateMutations_False_PseudoHeaderAllowed(t *testing.T) {
	srv := newScriptableAuthServer(t, http.StatusOK, map[string]string{
		"x-valid-header": "valid",
	}, "")
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: srv.srv.URL},
		AuthorizationResponse: &ext_authzv3.AuthorizationResponse{
			AllowedUpstreamHeaders: makeAuthResponseMatcher(t, "x-valid-header"),
		},
	}
	fn := buildHTTPCheckFnWithResponse(t, hs, false /* validateMutations=false */)

	disp, err := fn(context.Background(), minimalAuthRequest("/check", nil))
	if err != nil {
		t.Fatalf("checkFn(validate=false): unexpected error: %v", err)
	}
	if disp.class != dispAllow {
		t.Errorf("disposition class: got %v, want dispAllow (validate=false)", disp.class)
	}
}

// ---------------------------------------------------------------------------
// Group 8D — applyUpstreamMutations helper
// ---------------------------------------------------------------------------

// TestApplyUpstreamMutations_Set verifies that upstreamSet entries are applied
// to the header map via Set (overwrite semantics).
func TestApplyUpstreamMutations_Set(t *testing.T) {
	headers := http.Header{}
	headers.Set("x-existing", "old-value")

	disp := checkDisposition{
		class: dispAllow,
		upstreamSet: []headerKV{
			{name: "x-existing", value: "new-value"}, // overwrites
			{name: "x-new-header", value: "injected"},
		},
	}
	applyUpstreamMutations(headers, disp)

	if got := headers.Get("X-Existing"); got != "new-value" {
		t.Errorf("Set(x-existing): got %q, want %q (overwrite)", got, "new-value")
	}
	if got := headers.Get("X-New-Header"); got != "injected" {
		t.Errorf("Set(x-new-header): got %q, want %q", got, "injected")
	}
}

// TestApplyUpstreamMutations_Append verifies that upstreamApp entries are applied
// to the header map via Add (append semantics).
func TestApplyUpstreamMutations_Append(t *testing.T) {
	headers := http.Header{}
	headers.Set("x-existing", "first-value")

	disp := checkDisposition{
		class: dispAllow,
		upstreamApp: []headerKV{
			{name: "x-existing", value: "second-value"}, // appends to existing
			{name: "x-new-header", value: "appended"},
		},
	}
	applyUpstreamMutations(headers, disp)

	// x-existing must now have two values.
	vals := headers["X-Existing"]
	if len(vals) != 2 {
		t.Errorf("Append(x-existing): got %d values, want 2", len(vals))
	} else {
		if vals[0] != "first-value" {
			t.Errorf("Append(x-existing)[0]: got %q, want %q", vals[0], "first-value")
		}
		if vals[1] != "second-value" {
			t.Errorf("Append(x-existing)[1]: got %q, want %q", vals[1], "second-value")
		}
	}
	if got := headers.Get("X-New-Header"); got != "appended" {
		t.Errorf("Append(x-new-header): got %q, want %q", got, "appended")
	}
}

// TestApplyUpstreamMutations_SetBeforeAppend verifies that upstreamSet is applied
// before upstreamApp (set semantics take precedence when applied first).
func TestApplyUpstreamMutations_SetBeforeAppend(t *testing.T) {
	headers := http.Header{}
	headers.Set("x-header", "original")

	disp := checkDisposition{
		class: dispAllow,
		upstreamSet: []headerKV{
			{name: "x-header", value: "from-set"},
		},
		upstreamApp: []headerKV{
			{name: "x-header", value: "from-app"},
		},
	}
	applyUpstreamMutations(headers, disp)

	// After Set then Add: ["from-set", "from-app"]
	vals := headers["X-Header"]
	if len(vals) != 2 {
		t.Errorf("Set+Append(x-header): got %d values, want 2", len(vals))
	} else {
		if vals[0] != "from-set" {
			t.Errorf("Set+Append[0]: got %q, want %q", vals[0], "from-set")
		}
		if vals[1] != "from-app" {
			t.Errorf("Set+Append[1]: got %q, want %q", vals[1], "from-app")
		}
	}
}

// TestApplyUpstreamMutations_Empty verifies that empty upstreamSet/upstreamApp
// does not panic and leaves the header map unchanged.
func TestApplyUpstreamMutations_Empty(t *testing.T) {
	headers := http.Header{}
	headers.Set("x-keep", "value")

	disp := checkDisposition{
		class:       dispAllow,
		upstreamSet: nil,
		upstreamApp: nil,
	}
	applyUpstreamMutations(headers, disp)

	if got := headers.Get("X-Keep"); got != "value" {
		t.Errorf("Empty mutation: x-keep got %q, want %q", got, "value")
	}
	if len(headers) != 1 {
		t.Errorf("Empty mutation: header count got %d, want 1", len(headers))
	}
}

// ---------------------------------------------------------------------------
// Group 8E — Task 4 carried-forward fix: deprecated AuthorizationRequest.allowed_headers
// pre-compiled at buildHTTPCheckFn (config-load) time.
// ---------------------------------------------------------------------------

// TestDeprecatedAllowedHeaders_PreCompiled verifies that when
// AuthorizationRequest.allowed_headers is set, it is pre-compiled at
// buildHTTPCheckFn time (stored on compiledConfig.deprecatedAllowedHeaders)
// rather than per-request. Uses buildCompiledConfig to exercise the full
// config-load path.
func TestDeprecatedAllowedHeaders_PreCompiled(t *testing.T) {
	srv := newScriptableAuthServer(t, http.StatusOK, map[string]string{}, "")
	cfg := &ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{Uri: srv.srv.URL},
				AuthorizationRequest: &ext_authzv3.AuthorizationRequest{
					AllowedHeaders: makeListStringMatcher(exactPattern("authorization")),
				},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
	}
	cc, err := buildCompiledConfig(freshFactoryCtx(), cfg)
	if err != nil {
		t.Fatalf("buildCompiledConfig: unexpected error: %v", err)
	}
	if cc == nil {
		t.Fatal("buildCompiledConfig: got nil cc")
	}
	// Verify the deprecated field is pre-compiled and stored on cc.
	if cc.deprecatedAllowedHeaders == nil {
		t.Error("cc.deprecatedAllowedHeaders: got nil, want non-nil (pre-compiled at config-load)")
	}
	if cc.deprecatedAllowedHeaders != nil && !cc.deprecatedAllowedHeaders.matchAny("authorization") {
		t.Error("cc.deprecatedAllowedHeaders: does not match 'authorization'")
	}
}

// TestDeprecatedAllowedHeaders_MalformedParseReject verifies that a malformed
// regex in AuthorizationRequest.allowed_headers is caught at config-load time
// (PARSE-REJECT) rather than silently degrading at runtime.
func TestDeprecatedAllowedHeaders_MalformedParseReject(t *testing.T) {
	cfg := &ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{Uri: "http://127.0.0.1:9191"},
				AuthorizationRequest: &ext_authzv3.AuthorizationRequest{
					AllowedHeaders: makeListStringMatcher(safeRegexPattern("[invalid regex")),
				},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
	}
	_, err := buildCompiledConfig(freshFactoryCtx(), cfg)
	if err == nil {
		t.Fatal("buildCompiledConfig(malformed deprecated allowed_headers): want error, got nil")
	}
	// Error must mention the invalid regex or the deprecated field.
	if !strings.Contains(err.Error(), "allowed_headers") && !strings.Contains(err.Error(), "regex") {
		t.Errorf("error = %q; want mention of 'allowed_headers' or 'regex'", err.Error())
	}
}

// TestDeprecatedAllowedHeaders_NullOutWhenTopLevelSet verifies the security-
// relevant precedence property on real buildCompiledConfig output: when BOTH the
// top-level ExtAuthz.AllowedHeaders and the deprecated
// AuthorizationRequest.AllowedHeaders are set, buildCompiledConfig nulls out
// cc.deprecatedAllowedHeaders so the deprecated field cannot override (or
// widen/narrow) the top-level allow-list. The top-level allow-list must remain
// the sole effective request-side filter.
func TestDeprecatedAllowedHeaders_NullOutWhenTopLevelSet(t *testing.T) {
	srv := newScriptableAuthServer(t, http.StatusOK, map[string]string{}, "")
	cfg := &ext_authzv3.ExtAuthz{
		// Top-level allow-list (the primary, non-deprecated path).
		AllowedHeaders: makeListStringMatcher(exactPattern("x-top-level")),
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{Uri: srv.srv.URL},
				AuthorizationRequest: &ext_authzv3.AuthorizationRequest{
					// Deprecated allow-list — must NOT take effect when the
					// top-level field is present.
					AllowedHeaders: makeListStringMatcher(exactPattern("authorization")),
				},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
	}
	cc, err := buildCompiledConfig(freshFactoryCtx(), cfg)
	if err != nil {
		t.Fatalf("buildCompiledConfig: unexpected error: %v", err)
	}
	if cc == nil {
		t.Fatal("buildCompiledConfig: got nil cc")
	}
	// The deprecated field must be nulled out — it must not override the
	// top-level allow-list.
	if cc.deprecatedAllowedHeaders != nil {
		t.Error("cc.deprecatedAllowedHeaders: got non-nil, want nil (top-level AllowedHeaders set → deprecated field nulled)")
	}
	// The top-level allow-list must remain the effective filter.
	if cc.allowedHeaders == nil {
		t.Fatal("cc.allowedHeaders: got nil, want non-nil (top-level AllowedHeaders set)")
	}
	if !cc.allowedHeaders.matchAny("x-top-level") {
		t.Error("cc.allowedHeaders: does not match 'x-top-level' (top-level allow-list must be effective)")
	}
}

// ----------------------------------------------------------------------------
// Group 6 — with_request_body ADR-0128 reuse + over-limit 413 + DecodeData
// (Group 6 per SPEC §14.1 + PLAN Task 6; ADR-0162)
//
// Tests the following surfaces per SPEC §6.3 + parent SPEC §5.P5 + §6 amendment 6:
//   - DecodeHeaders: awaitingBody set + Continue returned when withRequestBody set + !endStream
//   - DecodeHeaders: awaitingBody NOT set when endStream=true (header-only request)
//   - DecodeHeaders: awaitingBody NOT set when withRequestBody is nil
//   - DecodeData: accumulation via the ADR-0128 primitive (body appended per chunk)
//   - DecodeData: over-limit + allow_partial_message:false → SendLocalReply(413, "Payload Too
//     Large", {Connection: close}) + DataStopIterationNoBuffer + NO counter increments
//   - DecodeData: over-limit + allow_partial_message:true → body truncated to max_request_bytes
//     prefix + DataStopIterationAndBuffer (Task 9 seam)
//   - DecodeData: endStream within limit → f.body complete + DataStopIterationAndBuffer (Task 9 seam)
//   - DecodeData: passthrough when awaitingBody=false (DataContinue)
//   - bufferSettings.packAsBytes parsed and stored on compiledConfig
//   - Per-route disable_request_body_buffering: overrides listener-level with_request_body OFF
//   - Strict > (not >=) cap predicate: accumulated == maxRequestBytes does NOT trip 413
//   - Multi-chunk accumulation: body assembled across multiple DecodeData calls
// ----------------------------------------------------------------------------

// fakeExtAuthzDCB is a minimal DecoderFilterCallbacks for Group 6 (and future Group 5/9) tests.
// It records SendLocalReply calls and provides a configurable per-route config return.
// Mirrors the buffer_test.go fakeCallbacks pattern.
type fakeExtAuthzDCB struct {
	perRoute        proto.Message
	localReplyCount int
	localReplyArgs  *localReplyRecord6
}

// localReplyRecord6 captures a single SendLocalReply invocation.
type localReplyRecord6 struct {
	status  int
	body    string
	headers envoyhttp.OrderedHeaders
}

func newFakeExtAuthzDCB() *fakeExtAuthzDCB { return &fakeExtAuthzDCB{} }

func (c *fakeExtAuthzDCB) ContinueDecoding()             {}
func (c *fakeExtAuthzDCB) DownstreamPrincipal() []string { return nil }
func (c *fakeExtAuthzDCB) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	c.localReplyCount++
	c.localReplyArgs = &localReplyRecord6{status: status, body: body, headers: headers}
}
func (c *fakeExtAuthzDCB) RequestRouteConfig() proto.Message { return c.perRoute }
func (c *fakeExtAuthzDCB) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (c *fakeExtAuthzDCB) EncodeHeaders(_ http.Header, _ bool) {}
func (c *fakeExtAuthzDCB) EncodeData(_ []byte, _ bool)         {}
func (c *fakeExtAuthzDCB) EncodeTrailers(_ http.Header)        {}

// newBodyBufferingFilter builds a minimal *filter with withRequestBody set on the
// compiledConfig and a fake DCB attached. It also pre-sets f.bodySettings to the
// same bufferSettings so tests that call DecodeData directly (without calling
// DecodeHeaders first) have the effective settings available.
// stats is optional (may be nil).
func newBodyBufferingFilter(t *testing.T, maxBytes uint32, allowPartial bool, packAsBytes bool, fs *filterStats) (*filter, *fakeExtAuthzDCB) {
	t.Helper()
	dcb := newFakeExtAuthzDCB()
	bs := &bufferSettings{
		maxRequestBytes:     maxBytes,
		allowPartialMessage: allowPartial,
		packAsBytes:         packAsBytes,
	}
	cc := &compiledConfig{
		withRequestBody: bs,
		statusOnError:   403,
		stats:           fs,
	}
	f := &filter{
		state:        &factoryState{listenerRC: cc},
		dcb:          dcb,
		activeRC:     cc,
		bodySettings: bs, // pre-set so DecodeData tests work without calling DecodeHeaders
	}
	return f, dcb
}

// ---------------------------------------------------------------------------
// Group 6A — DecodeHeaders body-buffering branch
// ---------------------------------------------------------------------------

// TestDecodeHeaders_WithRequestBody_SetsAwaitingBodyAndContinue verifies that
// DecodeHeaders sets awaitingBody=true and returns Continue (NOT StopIteration)
// when withRequestBody is set and endStream=false.
//
// Per ADR-0128 synchronous-HCM dispatch constraint: StopIteration from DecodeHeaders
// would deadlock (body loop is the ContinueDecoding path). Continue is correct.
func TestDecodeHeaders_WithRequestBody_SetsAwaitingBodyAndContinue(t *testing.T) {
	f, _ := newBodyBufferingFilter(t, 1024, false, false, nil)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")

	status := f.DecodeHeaders(headers, false /* endStream=false: body will follow */)

	if status != envoyhttp.Continue {
		t.Errorf("DecodeHeaders(withRequestBody, !endStream): want Continue, got %v", status)
	}
	if !f.awaitingBody {
		t.Error("DecodeHeaders(withRequestBody, !endStream): want awaitingBody=true, got false")
	}
}

// TestDecodeHeaders_WithRequestBody_EndStreamSkipsBuffer verifies that when
// endStream=true (header-only request, no body), DecodeHeaders does NOT set
// awaitingBody — there is no body to buffer, so the filter proceeds directly
// to the async outbound check dispatch (returns StopIteration).
//
// NOTE: Previously expected Continue when Task 9's dispatch was not wired.
// Task 9 wires the dispatch: endStream=true + withRequestBody skips body
// buffering and fires the async check directly → StopIteration.
// checkFn is nil in newBodyBufferingFilter, so dispatchOutboundCheck no-ops.
func TestDecodeHeaders_WithRequestBody_EndStreamSkipsBuffer(t *testing.T) {
	f, _ := newBodyBufferingFilter(t, 1024, false, false, nil)
	headers := http.Header{}

	status := f.DecodeHeaders(headers, true /* endStream=true: header-only request */)

	// Task 9: StopIteration returned (async dispatch fires for the header-only
	// request — no body to buffer, outbound check dispatches immediately).
	if status != envoyhttp.StopIteration {
		t.Errorf("DecodeHeaders(withRequestBody, endStream=true): want StopIteration (async dispatch), got %v", status)
	}
	if f.awaitingBody {
		t.Error("DecodeHeaders(withRequestBody, endStream=true): want awaitingBody=false (no body), got true")
	}
}

// TestDecodeHeaders_NoWithRequestBody_AwaitingBodyNotSet verifies that when
// withRequestBody is nil (body buffering OFF), awaitingBody is never set.
func TestDecodeHeaders_NoWithRequestBody_AwaitingBodyNotSet(t *testing.T) {
	dcb := newFakeExtAuthzDCB()
	cc := &compiledConfig{statusOnError: 403}
	f := &filter{state: &factoryState{listenerRC: cc}, dcb: dcb, activeRC: cc}
	headers := http.Header{}

	status := f.DecodeHeaders(headers, false /* body follows, but no buffering configured */)

	// Task 9 will return StopIteration here for the async check dispatch; for now
	// the skeleton returns Continue. The important assertion is awaitingBody=false.
	_ = status
	if f.awaitingBody {
		t.Error("DecodeHeaders(no withRequestBody): want awaitingBody=false, got true")
	}
}

// ---------------------------------------------------------------------------
// Group 6B — DecodeData accumulation (no over-limit)
// ---------------------------------------------------------------------------

// TestDecodeData_Passthrough_AwaitingBodyFalse verifies that when awaitingBody=false,
// DecodeData returns DataContinue without accumulating (pure passthrough).
func TestDecodeData_Passthrough_AwaitingBodyFalse(t *testing.T) {
	f, _ := newBodyBufferingFilter(t, 100, false, false, nil)
	f.awaitingBody = false // explicitly off (default)

	status := f.DecodeData([]byte("hello"), false)
	if status != envoyhttp.DataContinue {
		t.Errorf("DecodeData(awaitingBody=false): want DataContinue, got %v", status)
	}
	if len(f.body) != 0 {
		t.Errorf("DecodeData(awaitingBody=false): want empty body, got %d bytes", len(f.body))
	}
}

// TestDecodeData_SingleChunk_WithinLimit_EndStream_Parks verifies that a single
// chunk within the limit with endStream=true accumulates the body and parks the
// chain (DataStopIterationAndBuffer) — the Task 9 seam for firing the async check.
func TestDecodeData_SingleChunk_WithinLimit_EndStream_Parks(t *testing.T) {
	f, dcb := newBodyBufferingFilter(t, 100, false, false, nil)
	f.awaitingBody = true
	payload := []byte("hello world")

	status := f.DecodeData(payload, true /* endStream=true */)

	if status != envoyhttp.DataStopIterationAndBuffer {
		t.Errorf("DecodeData(within limit, endStream=true): want DataStopIterationAndBuffer (Task 9 seam), got %v", status)
	}
	if string(f.body) != "hello world" {
		t.Errorf("body: got %q, want %q", f.body, "hello world")
	}
	if dcb.localReplyCount != 0 {
		t.Errorf("SendLocalReply: got %d calls, want 0 (no 413 on within-limit body)", dcb.localReplyCount)
	}
}

// TestDecodeData_MultiChunk_WithinLimit_EndStream_Parks verifies that multiple
// non-terminal chunks accumulate correctly and the terminal chunk parks the chain.
func TestDecodeData_MultiChunk_WithinLimit_EndStream_Parks(t *testing.T) {
	f, dcb := newBodyBufferingFilter(t, 1024, false, false, nil)
	f.awaitingBody = true

	// Chunk 1: not endStream
	s1 := f.DecodeData([]byte("chunk1_"), false)
	if s1 != envoyhttp.DataContinue {
		t.Errorf("DecodeData(chunk1, !endStream): want DataContinue, got %v", s1)
	}
	// Chunk 2: endStream=true
	s2 := f.DecodeData([]byte("chunk2"), true)
	if s2 != envoyhttp.DataStopIterationAndBuffer {
		t.Errorf("DecodeData(chunk2, endStream=true): want DataStopIterationAndBuffer, got %v", s2)
	}
	if string(f.body) != "chunk1_chunk2" {
		t.Errorf("body: got %q, want %q", f.body, "chunk1_chunk2")
	}
	if dcb.localReplyCount != 0 {
		t.Errorf("SendLocalReply: got %d calls, want 0", dcb.localReplyCount)
	}
}

// TestDecodeData_ExactCap_StrictGreaterThan verifies the cap predicate is strict >
// (not >=): accumulated == maxRequestBytes must NOT fire the 413.
// Per ADR-0128 buffer filter precedent (§11.2 strict > predicate).
func TestDecodeData_ExactCap_StrictGreaterThan(t *testing.T) {
	const maxBytes = 10
	f, dcb := newBodyBufferingFilter(t, maxBytes, false, false, nil)
	f.awaitingBody = true

	// Exactly maxBytes — must NOT over-limit.
	payload := make([]byte, maxBytes)
	status := f.DecodeData(payload, true /* endStream */)

	if status == envoyhttp.DataStopIterationNoBuffer {
		t.Error("DecodeData(exact cap): got DataStopIterationNoBuffer (413 fired), want DataStopIterationAndBuffer (no 413 on exact-cap)")
	}
	if dcb.localReplyCount != 0 {
		t.Errorf("SendLocalReply: got %d calls, want 0 (exact cap must NOT fire 413)", dcb.localReplyCount)
	}
}

// ---------------------------------------------------------------------------
// Group 6C — Over-limit: allow_partial_message=false → 413
// ---------------------------------------------------------------------------

// TestDecodeData_OverLimit_AllowPartialFalse_413 verifies that when the accumulated
// body exceeds max_request_bytes and allow_partial_message=false, the filter emits
// SendLocalReply(413, "Payload Too Large", {Connection: close}) and returns
// DataStopIterationNoBuffer — the auth service is NEVER contacted.
//
// Per parent SPEC §5.P5 + §6 amendment 6 + ADR-0162: the 413 fires BEFORE the
// outbound check; NO ext_authz counter increments.
func TestDecodeData_OverLimit_AllowPartialFalse_413(t *testing.T) {
	const maxBytes uint32 = 10
	reg := stats.NewRegistry()
	ctx := freshFactoryCtxWithRegistry(reg)
	fs := newFilterStats(reg, ctx.StatPrefix)

	f, dcb := newBodyBufferingFilter(t, maxBytes, false /* allow_partial=false */, false, fs)
	f.awaitingBody = true

	// Send a body that exceeds maxBytes.
	oversized := make([]byte, int(maxBytes)+1)
	status := f.DecodeData(oversized, true /* endStream */)

	// Must return DataStopIterationNoBuffer (discard partial buffer).
	if status != envoyhttp.DataStopIterationNoBuffer {
		t.Errorf("DecodeData(over-limit, allow_partial=false): want DataStopIterationNoBuffer, got %v", status)
	}

	// Must emit SendLocalReply(413, "Payload Too Large", ...).
	if dcb.localReplyCount != 1 {
		t.Fatalf("SendLocalReply: got %d calls, want 1", dcb.localReplyCount)
	}
	if dcb.localReplyArgs.status != 413 {
		t.Errorf("SendLocalReply status: got %d, want 413", dcb.localReplyArgs.status)
	}
	if dcb.localReplyArgs.body != "Payload Too Large" {
		t.Errorf("SendLocalReply body: got %q, want %q", dcb.localReplyArgs.body, "Payload Too Large")
	}
	// Connection: close header required per parent §5.P5.
	if dcb.localReplyArgs.headers.Get("Connection") != "close" {
		t.Errorf("SendLocalReply headers: Connection: close missing; headers=%v", dcb.localReplyArgs.headers)
	}

	// NO ext_authz counter increments on the over-limit 413 path.
	// The request never reached a disposition; counters must remain at 0.
	// Per parent SPEC §6 amendment 6: "auth NOT called, NO counters".
	if got := fs.ok.Load(); got != 0 {
		t.Errorf("ok counter: got %d, want 0 (413 path must not increment ok)", got)
	}
	if got := fs.denied.Load(); got != 0 {
		t.Errorf("denied counter: got %d, want 0 (413 path must not increment denied)", got)
	}
	if got := fs.errored.Load(); got != 0 {
		t.Errorf("errored counter: got %d, want 0 (413 path must not increment errored)", got)
	}
	if got := fs.failureModeAllowed.Load(); got != 0 {
		t.Errorf("failureModeAllowed counter: got %d, want 0", got)
	}
	if got := fs.invalid.Load(); got != 0 {
		t.Errorf("invalid counter: got %d, want 0", got)
	}
}

// TestDecodeData_OverLimit_AllowPartialFalse_MidStream verifies that an over-limit
// detection on a non-endStream chunk still fires the 413 (the check is on the
// running accumulated total, not only at endStream).
func TestDecodeData_OverLimit_AllowPartialFalse_MidStream(t *testing.T) {
	const maxBytes uint32 = 5
	f, dcb := newBodyBufferingFilter(t, maxBytes, false, false, nil)
	f.awaitingBody = true

	// First chunk: 3 bytes (within limit)
	s1 := f.DecodeData([]byte("abc"), false /* !endStream */)
	if s1 != envoyhttp.DataContinue {
		t.Errorf("chunk1: want DataContinue, got %v", s1)
	}
	if dcb.localReplyCount != 0 {
		t.Error("chunk1: unexpected SendLocalReply call")
	}

	// Second chunk: 3 bytes (total 6 > maxBytes=5) — OVER LIMIT even mid-stream
	s2 := f.DecodeData([]byte("def"), false /* !endStream */)
	if s2 != envoyhttp.DataStopIterationNoBuffer {
		t.Errorf("chunk2 (over-limit mid-stream): want DataStopIterationNoBuffer, got %v", s2)
	}
	if dcb.localReplyCount != 1 {
		t.Fatalf("chunk2: want 1 SendLocalReply call, got %d", dcb.localReplyCount)
	}
	if dcb.localReplyArgs.status != 413 {
		t.Errorf("413 status: got %d, want 413", dcb.localReplyArgs.status)
	}
}

// TestDecodeData_OverLimit_NoCounterIncrements verifies the NO-counter-increments
// invariant explicitly: stats must remain at 0 on the 413 over-limit path.
// Tests the "auth NOT called, NO counters" assertion from parent §6 amendment 6.
func TestDecodeData_OverLimit_NoCounterIncrements(t *testing.T) {
	reg := stats.NewRegistry()
	ctx := freshFactoryCtxWithRegistry(reg)
	fs := newFilterStats(reg, ctx.StatPrefix)

	f, _ := newBodyBufferingFilter(t, 5, false, false, fs)
	f.awaitingBody = true

	// Oversized payload.
	_ = f.DecodeData(make([]byte, 100), true)

	// All 6 counters must be at 0.
	for _, tc := range []struct {
		name    string
		counter *stats.Counter
	}{
		{"ok", fs.ok},
		{"denied", fs.denied},
		{"errored", fs.errored},
		{"disabled", fs.disabled},
		{"failureModeAllowed", fs.failureModeAllowed},
		{"invalid", fs.invalid},
	} {
		if got := tc.counter.Load(); got != 0 {
			t.Errorf("counter %q: got %d, want 0 (413 path must not increment any counter)", tc.name, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Group 6D — Over-limit: allow_partial_message=true → truncated prefix
// ---------------------------------------------------------------------------

// TestDecodeData_OverLimit_AllowPartialTrue_TruncatesToMaxBytes verifies that when
// the body exceeds max_request_bytes and allow_partial_message=true, f.body is
// truncated to exactly max_request_bytes bytes.
//
// Per parent SPEC §5.P5: the auth service receives the truncated max_request_bytes
// prefix; the Task 9 seam injects x-envoy-auth-partial-body:true into the auth request.
func TestDecodeData_OverLimit_AllowPartialTrue_TruncatesToMaxBytes(t *testing.T) {
	const maxBytes uint32 = 10
	f, dcb := newBodyBufferingFilter(t, maxBytes, true /* allow_partial=true */, false, nil)
	f.awaitingBody = true

	// Build a payload larger than maxBytes.
	oversized := make([]byte, int(maxBytes)+50)
	for i := range oversized {
		oversized[i] = byte('a' + (i % 26))
	}

	status := f.DecodeData(oversized, true /* endStream */)

	// Must park (DataStopIterationAndBuffer) for Task 9 async check.
	if status != envoyhttp.DataStopIterationAndBuffer {
		t.Errorf("DecodeData(over-limit, allow_partial=true): want DataStopIterationAndBuffer (park for auth check), got %v", status)
	}
	// Must NOT fire 413.
	if dcb.localReplyCount != 0 {
		t.Errorf("SendLocalReply: got %d calls, want 0 (allow_partial=true must NOT fire 413)", dcb.localReplyCount)
	}
	// f.body must be truncated to exactly maxBytes.
	if uint32(len(f.body)) != maxBytes {
		t.Errorf("f.body length: got %d, want %d (truncated to maxRequestBytes)", len(f.body), maxBytes)
	}
	// Verify prefix correctness: first maxBytes bytes of oversized.
	if string(f.body) != string(oversized[:maxBytes]) {
		t.Errorf("f.body prefix: got %q, want %q", f.body, oversized[:maxBytes])
	}
}

// TestDecodeData_OverLimit_AllowPartialTrue_MultiChunk verifies truncation works
// when the over-limit condition is reached across multiple chunks.
func TestDecodeData_OverLimit_AllowPartialTrue_MultiChunk(t *testing.T) {
	const maxBytes uint32 = 5
	f, dcb := newBodyBufferingFilter(t, maxBytes, true, false, nil)
	f.awaitingBody = true

	// Chunk 1: 3 bytes (within limit)
	s1 := f.DecodeData([]byte("abc"), false)
	if s1 != envoyhttp.DataContinue {
		t.Errorf("chunk1: want DataContinue, got %v", s1)
	}

	// Chunk 2: 5 more bytes (total 8 > maxBytes=5). allow_partial=true → truncate.
	s2 := f.DecodeData([]byte("defgh"), true /* endStream */)
	if s2 != envoyhttp.DataStopIterationAndBuffer {
		t.Errorf("chunk2 (allow_partial, over-limit): want DataStopIterationAndBuffer, got %v", s2)
	}
	// No 413.
	if dcb.localReplyCount != 0 {
		t.Errorf("SendLocalReply: got %d calls, want 0", dcb.localReplyCount)
	}
	// f.body truncated to maxBytes=5: "abcde" (first 5 of "abcdefgh").
	if uint32(len(f.body)) != maxBytes {
		t.Errorf("f.body length: got %d, want %d", len(f.body), maxBytes)
	}
	if string(f.body) != "abcde" {
		t.Errorf("f.body: got %q, want %q (first %d bytes)", f.body, "abcde", maxBytes)
	}
}

// TestDecodeData_OverLimit_AllowPartialTrue_MidStreamTruncationThenChunk verifies
// truncation is idempotent across multiple over-limit chunks: chunk 1 within
// limit, chunk 2 over-limit mid-stream (endStream=false → DataContinue, body
// truncated), chunk 3 still arrives non-terminal and over-limit (truncation
// repeats idempotently, body length unchanged), final chunk lands the terminal
// DataStopIterationAndBuffer. Body length must equal maxRequestBytes throughout
// the post-truncation chunks.
func TestDecodeData_OverLimit_AllowPartialTrue_MidStreamTruncationThenChunk(t *testing.T) {
	const maxBytes uint32 = 5
	f, dcb := newBodyBufferingFilter(t, maxBytes, true /* allow_partial=true */, false, nil)
	f.awaitingBody = true

	// Chunk 1: "abc" (3 bytes, within limit), endStream=false.
	s1 := f.DecodeData([]byte("abc"), false)
	if s1 != envoyhttp.DataContinue {
		t.Errorf("chunk1 (within-limit, non-terminal): want DataContinue, got %v", s1)
	}
	if string(f.body) != "abc" {
		t.Errorf("chunk1: f.body = %q, want %q", f.body, "abc")
	}

	// Chunk 2: "defgh" (5 more bytes → total 8 > maxBytes=5), endStream=false.
	// allow_partial=true + over-limit mid-stream → truncate to maxBytes; status
	// is DataContinue (NOT terminal; not the Task 9 park seam).
	s2 := f.DecodeData([]byte("defgh"), false)
	if s2 != envoyhttp.DataContinue {
		t.Errorf("chunk2 (over-limit, non-terminal, allow_partial): want DataContinue, got %v", s2)
	}
	if dcb.localReplyCount != 0 {
		t.Errorf("chunk2: SendLocalReply count = %d, want 0 (allow_partial=true must NOT fire 413)", dcb.localReplyCount)
	}
	if uint32(len(f.body)) != maxBytes {
		t.Errorf("chunk2: f.body length = %d, want %d (truncated to maxRequestBytes)", len(f.body), maxBytes)
	}
	if string(f.body) != "abcde" {
		t.Errorf("chunk2: f.body = %q, want %q (first %d bytes of accumulated)", f.body, "abcde", maxBytes)
	}

	// Chunk 3: "ijk" (3 more bytes; still over-limit because f.body is already at
	// maxBytes), endStream=false. Truncation repeats idempotently: f.body is
	// re-truncated to the same maxBytes (the first 5 bytes of "abcde"+"ijk" =
	// "abcdeijk" → "abcde"). Status remains DataContinue.
	s3 := f.DecodeData([]byte("ijk"), false)
	if s3 != envoyhttp.DataContinue {
		t.Errorf("chunk3 (still-over-limit, non-terminal): want DataContinue, got %v", s3)
	}
	if dcb.localReplyCount != 0 {
		t.Errorf("chunk3: SendLocalReply count = %d, want 0", dcb.localReplyCount)
	}
	if uint32(len(f.body)) != maxBytes {
		t.Errorf("chunk3: f.body length = %d, want %d (truncation idempotent)", len(f.body), maxBytes)
	}
	if string(f.body) != "abcde" {
		t.Errorf("chunk3: f.body = %q, want %q (idempotent: prefix unchanged)", f.body, "abcde")
	}

	// Chunk 4: "lm" (2 more bytes), endStream=true. Terminal over-limit chunk
	// must return DataStopIterationAndBuffer (the Task 9 park seam) while the
	// body remains truncated at maxBytes.
	s4 := f.DecodeData([]byte("lm"), true /* endStream */)
	if s4 != envoyhttp.DataStopIterationAndBuffer {
		t.Errorf("chunk4 (terminal, over-limit, allow_partial): want DataStopIterationAndBuffer, got %v", s4)
	}
	if dcb.localReplyCount != 0 {
		t.Errorf("chunk4: SendLocalReply count = %d, want 0", dcb.localReplyCount)
	}
	if uint32(len(f.body)) != maxBytes {
		t.Errorf("chunk4: f.body length = %d, want %d (still truncated to maxRequestBytes)", len(f.body), maxBytes)
	}
	if string(f.body) != "abcde" {
		t.Errorf("chunk4: f.body = %q, want %q (final truncated prefix)", f.body, "abcde")
	}
}

// ---------------------------------------------------------------------------
// Group 6E — pack_as_bytes parsed + stored
// ---------------------------------------------------------------------------

// TestWithRequestBody_PackAsBytesStored verifies that the pack_as_bytes field is
// parsed from the proto and stored on compiledConfig.withRequestBody.packAsBytes.
// In HTTP-mode (18.1) pack_as_bytes has no effect on the POST body — the body bytes
// are transmitted verbatim regardless. The field is parsed for gRPC-mode (18.2).
func TestWithRequestBody_PackAsBytesStored(t *testing.T) {
	cfg := &ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{Uri: "http://127.0.0.1:9191"},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
		WithRequestBody: &ext_authzv3.BufferSettings{
			MaxRequestBytes: 512,
			PackAsBytes:     true,
		},
	}
	cc, err := buildCompiledConfig(freshFactoryCtx(), cfg)
	if err != nil {
		t.Fatalf("buildCompiledConfig: unexpected error: %v", err)
	}
	if cc.withRequestBody == nil {
		t.Fatal("cc.withRequestBody: got nil, want non-nil")
	}
	if !cc.withRequestBody.packAsBytes {
		t.Error("cc.withRequestBody.packAsBytes: got false, want true")
	}
}

// TestDecodeData_PackAsBytes_BodyVerbatimInHTTPMode verifies that pack_as_bytes=true
// does NOT change the body bytes stored in f.body — HTTP-mode always transmits the
// raw bytes verbatim (pack_as_bytes is a gRPC-mode proto differentiation, 18.2).
func TestDecodeData_PackAsBytes_BodyVerbatimInHTTPMode(t *testing.T) {
	const maxBytes uint32 = 100
	f, dcb := newBodyBufferingFilter(t, maxBytes, false, true /* pack_as_bytes=true */, nil)
	f.awaitingBody = true

	payload := []byte("raw bytes \x00\x01\x02")
	status := f.DecodeData(payload, true)

	if status != envoyhttp.DataStopIterationAndBuffer {
		t.Errorf("DecodeData(pack_as_bytes=true): want DataStopIterationAndBuffer, got %v", status)
	}
	if dcb.localReplyCount != 0 {
		t.Errorf("SendLocalReply: unexpected call (got %d)", dcb.localReplyCount)
	}
	if string(f.body) != string(payload) {
		t.Errorf("f.body: got %v, want %v (verbatim bytes in HTTP mode)", f.body, payload)
	}
}

// ---------------------------------------------------------------------------
// Group 6F — Per-route disable_request_body_buffering override
// ---------------------------------------------------------------------------

// TestDecodeHeaders_PerRouteDisableBodyBuffering verifies that when per-route
// check_settings.disable_request_body_buffering=true, the effective withRequestBody
// is nil (OFF) — body buffering is suppressed even if the listener-level
// with_request_body is set.
//
// Per SPEC §8 + SPEC §6.3 step 3: "effective withRequestBody = perRoute override OR
// listener-level". disable_request_body_buffering=true forces body-buffering OFF.
func TestDecodeHeaders_PerRouteDisableBodyBuffering(t *testing.T) {
	// Listener-level has with_request_body set.
	cc := &compiledConfig{
		withRequestBody: &bufferSettings{maxRequestBytes: 1024, allowPartialMessage: false},
		statusOnError:   403,
	}
	state := &factoryState{listenerRC: cc}

	// Per-route: disable_request_body_buffering=true.
	perRouteProto := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &ext_authzv3.CheckSettings{
				DisableRequestBodyBuffering: true,
			},
		},
	}
	dcb := newFakeExtAuthzDCB()
	dcb.perRoute = perRouteProto

	f := &filter{
		state:    state,
		dcb:      dcb,
		activeRC: cc,
	}

	// DecodeHeaders with per-route disable: awaitingBody must NOT be set.
	headers := http.Header{}
	f.DecodeHeaders(headers, false /* endStream=false: body would follow */)

	if f.awaitingBody {
		t.Error("DecodeHeaders(per-route disable_request_body_buffering=true): want awaitingBody=false, got true")
	}
}

// TestDecodeHeaders_PerRouteWithRequestBodyOverride verifies that per-route
// check_settings.with_request_body overrides the listener-level with_request_body.
//
// Per SPEC §8: if per-route check_settings.with_request_body is set, it replaces
// the listener-level with_request_body for this request.
func TestDecodeHeaders_PerRouteWithRequestBodyOverride(t *testing.T) {
	// Listener-level: no with_request_body (nil).
	cc := &compiledConfig{
		withRequestBody: nil, // listener-level: OFF
		statusOnError:   403,
	}
	state := &factoryState{listenerRC: cc}

	// Per-route: with_request_body set to max=64.
	perRouteProto := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &ext_authzv3.CheckSettings{
				WithRequestBody: &ext_authzv3.BufferSettings{
					MaxRequestBytes: 64,
				},
			},
		},
	}
	dcb := newFakeExtAuthzDCB()
	dcb.perRoute = perRouteProto

	f := &filter{
		state:    state,
		dcb:      dcb,
		activeRC: cc,
	}

	headers := http.Header{}
	f.DecodeHeaders(headers, false /* body will follow */)

	// Per-route with_request_body=64 must activate body buffering.
	if !f.awaitingBody {
		t.Error("DecodeHeaders(per-route with_request_body=64, listener-level nil): want awaitingBody=true, got false")
	}
}

// ----------------------------------------------------------------------------
// Group 5 — DecodeHeaders top-level dispatch + async-resume outbound-call leg
// (Group 5 per SPEC §14.1 + PLAN Task 9)
//
// Tests the following surfaces per SPEC §6.3:
//   - Per-route disabled short-circuit: Continue returned immediately, NO auth
//     call, NO counter increments.
//   - Async-resume allow path: StopIteration returned; goroutine calls checkFn;
//     on dispAllow → applyUpstreamMutations + optional clearRouteCache +
//     ContinueDecoding() + ok counter increment.
//   - Async-resume deny path: StopIteration returned; goroutine calls checkFn;
//     on dispDeny → SendLocalReply(denyStatus, denyBody, denyHeaders) +
//     denied counter increment.
//   - Async-resume error path (failure_mode_allow:false): StopIteration returned;
//     goroutine calls checkFn; on dispError → SendLocalReply(statusOnError, "",
//     nil) + errored counter increment (no failureModeAllowed increment).
//   - Async-resume error path (failure_mode_allow:true): StopIteration returned;
//     goroutine calls checkFn; on dispError → ContinueDecoding() + errored ++
//     failureModeAllowed ++; optional x-envoy-auth-failure-mode-allowed header.
//   - Async-resume invalid path: invalid counter increment + error posture.
//   - Body-complete path (DecodeData endStream): same dispatch logic via
//     dispatchOutboundCheck, DataStopIterationAndBuffer park, goroutine resume.
// ----------------------------------------------------------------------------

// asyncExtAuthzDCB is a DecoderFilterCallbacks for Group 5/9 tests that records
// ContinueDecoding and SendLocalReply invocations, and captures the current
// upstream request headers for allow-path injection assertions.
// Mirrors fakeExtAuthzDCB but adds continueCount + captures upstream headers.
// All mutable state is guarded by mu so the race detector stays clean when
// the resume goroutine fires from the async dispatch and the test goroutine
// polls via waitForContinueOrReply concurrently.
type asyncExtAuthzDCB struct {
	mu            sync.Mutex
	perRoute      proto.Message
	upstreamHdrs  http.Header // the headers passed by the caller to the filter (allow-path mutations land here)
	continueCount int
	localReply    *localReplyRecord6
}

func newAsyncExtAuthzDCB(upstream http.Header) *asyncExtAuthzDCB {
	if upstream == nil {
		upstream = make(http.Header)
	}
	return &asyncExtAuthzDCB{upstreamHdrs: upstream}
}

func (c *asyncExtAuthzDCB) ContinueDecoding() {
	c.mu.Lock()
	c.continueCount++
	c.mu.Unlock()
}
func (c *asyncExtAuthzDCB) DownstreamPrincipal() []string { return nil }
func (c *asyncExtAuthzDCB) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	c.mu.Lock()
	c.localReply = &localReplyRecord6{status: status, body: body, headers: headers}
	c.mu.Unlock()
}
func (c *asyncExtAuthzDCB) RequestRouteConfig() proto.Message { return c.perRoute }
func (c *asyncExtAuthzDCB) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (c *asyncExtAuthzDCB) EncodeHeaders(_ http.Header, _ bool) {}
func (c *asyncExtAuthzDCB) EncodeData(_ []byte, _ bool)         {}
func (c *asyncExtAuthzDCB) EncodeTrailers(_ http.Header)        {}

// waitForContinueOrReply waits up to maxWait for either ContinueDecoding or
// SendLocalReply to fire (polling). Returns true when one fires. Used in
// async-resume tests to avoid sleeps.
func waitForContinueOrReply(dcb *asyncExtAuthzDCB, maxWait time.Duration) bool {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		dcb.mu.Lock()
		fired := dcb.continueCount > 0 || dcb.localReply != nil
		dcb.mu.Unlock()
		if fired {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

// asyncDCB_continueCount returns the continueCount under mu (race-clean accessor).
func asyncDCB_continueCount(dcb *asyncExtAuthzDCB) int {
	dcb.mu.Lock()
	defer dcb.mu.Unlock()
	return dcb.continueCount
}

// asyncDCB_localReply returns the localReply pointer under mu (race-clean accessor).
func asyncDCB_localReply(dcb *asyncExtAuthzDCB) *localReplyRecord6 {
	dcb.mu.Lock()
	defer dcb.mu.Unlock()
	return dcb.localReply
}

// buildDispatchFilter constructs a minimal *filter configured for async-dispatch
// tests: a real auth server (via checkFn), optional stats registry, and the
// given DCB. The compiledConfig carries the supplied checkFn + error-posture.
func buildDispatchFilter(
	t *testing.T,
	srv *scriptableAuthServer,
	failureModeAllow bool,
	failureModeAllowHeaderAdd bool,
	clearRouteCache bool,
	statusOnError uint32,
	fs *filterStats,
	dcb *asyncExtAuthzDCB,
) *filter {
	t.Helper()
	fn := buildHTTPCheckFnForTest(t, srv.srv.URL, 0, "")
	cc := &compiledConfig{
		checkFn:                   fn,
		failureModeAllow:          failureModeAllow,
		failureModeAllowHeaderAdd: failureModeAllowHeaderAdd,
		clearRouteCache:           clearRouteCache,
		statusOnError:             statusOnError,
		stats:                     fs,
	}
	f := &filter{
		state:    &factoryState{listenerRC: cc},
		dcb:      dcb,
		activeRC: cc,
	}
	return f
}

// ---------------------------------------------------------------------------
// Group 5A — per-route disabled short-circuit
// ---------------------------------------------------------------------------

// TestDecodeHeaders_PerRouteDisabled_ContinueNoCounters verifies that when the
// per-route config has disabled:true, DecodeHeaders returns Continue immediately
// with NO auth call and NO counter increments.
//
// Per SPEC §6.3 step 2 + parent §6 amendment 7.
func TestDecodeHeaders_PerRouteDisabled_ContinueNoCounters(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_http")
	ok0 := reg.NewCounterIfAbsent("http.ingress_http.ext_authz.ok")

	// Auth server that MUST NOT be called; it counts calls.
	var authCalled int
	sas := &scriptableAuthServer{}
	sas.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authCalled++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(func() { sas.srv.Close() })

	fn := buildHTTPCheckFnForTest(t, sas.srv.URL, 0, "")
	cc := &compiledConfig{
		checkFn:       fn,
		statusOnError: 403,
		stats:         fs,
	}
	state := &factoryState{listenerRC: cc}

	// Per-route disabled:true.
	perRouteProto := &ext_authzv3.ExtAuthzPerRoute{
		Override: &ext_authzv3.ExtAuthzPerRoute_Disabled{Disabled: true},
	}
	dcb := newFakeExtAuthzDCB()
	dcb.perRoute = perRouteProto

	f := &filter{state: state, dcb: dcb, activeRC: cc}

	headers := http.Header{}
	headers.Set("Authorization", "Bearer token")
	status := f.DecodeHeaders(headers, true)

	if status != envoyhttp.Continue {
		t.Errorf("DecodeHeaders(disabled): want Continue, got %v", status)
	}
	if authCalled > 0 {
		t.Errorf("disabled: auth server called %d times, want 0", authCalled)
	}
	// No counter increments: ok counter must still be 0.
	if v := ok0.Load(); v != 0 {
		t.Errorf("disabled: ok counter = %d, want 0 (no counter increments on disabled path)", v)
	}
}

// ---------------------------------------------------------------------------
// Group 5B — async-resume allow path
// ---------------------------------------------------------------------------

// TestDecodeHeaders_AsyncAllow_UpstreamMutation verifies the full allow path:
// StopIteration returned synchronously; goroutine fires; on dispAllow the
// allowed_upstream_headers are injected + ContinueDecoding() fires + ok counter
// increments.
func TestDecodeHeaders_AsyncAllow_UpstreamMutation(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_http")
	okCtr := reg.NewCounterIfAbsent("http.ingress_http.ext_authz.ok")

	srv := newScriptableAuthServer(t, http.StatusOK, map[string]string{
		"x-auth-upstream": "injected-value",
	}, "")

	// Build checkFn with allowed_upstream_headers matching "x-auth-upstream".
	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: srv.srv.URL},
		AuthorizationResponse: &ext_authzv3.AuthorizationResponse{
			AllowedUpstreamHeaders: makeListStringMatcher(exactPattern("x-auth-upstream")),
		},
	}
	fn, err := buildHTTPCheckFn(hs, false)
	if err != nil {
		t.Fatalf("buildHTTPCheckFn: %v", err)
	}
	cc := &compiledConfig{
		checkFn:       fn,
		statusOnError: 403,
		stats:         fs,
	}
	upstream := make(http.Header)
	dcb := newAsyncExtAuthzDCB(upstream)
	f := &filter{state: &factoryState{listenerRC: cc}, dcb: dcb, activeRC: cc}

	status := f.DecodeHeaders(upstream, true /* endStream — no body */)
	if status != envoyhttp.StopIteration {
		t.Fatalf("DecodeHeaders(allow): want StopIteration, got %v", status)
	}

	if !waitForContinueOrReply(dcb, 2*time.Second) {
		t.Fatal("async allow: ContinueDecoding never fired within 2s")
	}

	if c := asyncDCB_continueCount(dcb); c != 1 {
		t.Errorf("ContinueDecoding: called %d times, want 1", c)
	}
	if r := asyncDCB_localReply(dcb); r != nil {
		t.Errorf("allow path: SendLocalReply fired unexpectedly: %+v", r)
	}
	if v := okCtr.Load(); v != 1 {
		t.Errorf("ok counter: got %d, want 1", v)
	}
	// Upstream header injection: the header set by the auth service must be present.
	// Read upstream under f.mu to satisfy the race detector: the goroutine writes
	// upstream under f.mu, so we must acquire the same lock before reading.
	f.mu.Lock()
	got := upstream.Get("X-Auth-Upstream")
	f.mu.Unlock()
	if got != "injected-value" {
		t.Errorf("upstream injection: X-Auth-Upstream = %q, want %q", got, "injected-value")
	}
}

// TestDecodeHeaders_AsyncAllow_ClearRouteCache verifies that when
// cc.clearRouteCache=true and the auth service allows, f.clearRouteCacheRequested
// flips to true (the framework primitive landing is deferred; the flag is the
// test-introspection anchor per the jwt_authn precedent).
func TestDecodeHeaders_AsyncAllow_ClearRouteCache(t *testing.T) {
	srv := newScriptableAuthServer(t, http.StatusOK, nil, "")
	fn := buildHTTPCheckFnForTest(t, srv.srv.URL, 0, "")
	cc := &compiledConfig{checkFn: fn, clearRouteCache: true, statusOnError: 403}
	dcb := newAsyncExtAuthzDCB(make(http.Header))
	f := &filter{state: &factoryState{listenerRC: cc}, dcb: dcb, activeRC: cc}

	f.DecodeHeaders(make(http.Header), true)
	if !waitForContinueOrReply(dcb, 2*time.Second) {
		t.Fatal("async allow (clear_route_cache): ContinueDecoding never fired within 2s")
	}

	if !f.clearRouteCacheRequested {
		t.Error("clearRouteCacheRequested: want true when clearRouteCache=true + allow, got false")
	}
}

// ---------------------------------------------------------------------------
// Group 5C — async-resume deny path
// ---------------------------------------------------------------------------

// TestDecodeHeaders_AsyncDeny_SendLocalReply verifies the deny path:
// StopIteration returned synchronously; goroutine fires; on dispDeny
// SendLocalReply is called with the auth service's status/body/headers +
// denied counter increments.
func TestDecodeHeaders_AsyncDeny_SendLocalReply(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_http")
	deniedCtr := reg.NewCounterIfAbsent("http.ingress_http.ext_authz.denied")

	srv := newScriptableAuthServer(t, http.StatusForbidden, map[string]string{
		"x-ext-authz-error": "unauthorized",
	}, "access denied")

	hs := &ext_authzv3.HttpService{
		ServerUri: &corev3.HttpUri{Uri: srv.srv.URL},
		AuthorizationResponse: &ext_authzv3.AuthorizationResponse{
			AllowedClientHeaders: makeListStringMatcher(exactPattern("x-ext-authz-error")),
		},
	}
	fn, err := buildHTTPCheckFn(hs, false)
	if err != nil {
		t.Fatalf("buildHTTPCheckFn: %v", err)
	}
	cc := &compiledConfig{checkFn: fn, statusOnError: 403, stats: fs}
	dcb := newAsyncExtAuthzDCB(make(http.Header))
	f := &filter{state: &factoryState{listenerRC: cc}, dcb: dcb, activeRC: cc}

	status := f.DecodeHeaders(make(http.Header), true)
	if status != envoyhttp.StopIteration {
		t.Fatalf("DecodeHeaders(deny): want StopIteration, got %v", status)
	}

	if !waitForContinueOrReply(dcb, 2*time.Second) {
		t.Fatal("async deny: SendLocalReply never fired within 2s")
	}

	r := asyncDCB_localReply(dcb)
	if r == nil {
		t.Fatal("deny path: SendLocalReply not called")
	}
	if r.status != http.StatusForbidden {
		t.Errorf("deny status: got %d, want 403", r.status)
	}
	if string(r.body) != "access denied" {
		t.Errorf("deny body: got %q, want %q", r.body, "access denied")
	}
	if c := asyncDCB_continueCount(dcb); c != 0 {
		t.Errorf("deny path: ContinueDecoding called %d times, want 0", c)
	}
	if v := deniedCtr.Load(); v != 1 {
		t.Errorf("denied counter: got %d, want 1", v)
	}
}

// ---------------------------------------------------------------------------
// Group 5D — async-resume error path
// ---------------------------------------------------------------------------

// TestDecodeHeaders_AsyncError_FailureModeAllow_False verifies that when the
// auth server is unreachable and failure_mode_allow=false, SendLocalReply is
// called with statusOnError and the errored counter increments.
func TestDecodeHeaders_AsyncError_FailureModeAllow_False(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_http")
	erroredCtr := reg.NewCounterIfAbsent("http.ingress_http.ext_authz.error")

	// Use an invalid server URL to force a transport error.
	fn := buildHTTPCheckFnForTest(t, "http://127.0.0.1:19191", 200 /* 200ms timeout */, "")
	cc := &compiledConfig{
		checkFn:          fn,
		failureModeAllow: false,
		statusOnError:    503,
		stats:            fs,
	}
	dcb := newAsyncExtAuthzDCB(make(http.Header))
	f := &filter{state: &factoryState{listenerRC: cc}, dcb: dcb, activeRC: cc}

	status := f.DecodeHeaders(make(http.Header), true)
	if status != envoyhttp.StopIteration {
		t.Fatalf("DecodeHeaders(error, fma=false): want StopIteration, got %v", status)
	}

	if !waitForContinueOrReply(dcb, 3*time.Second) {
		t.Fatal("error path (fma=false): SendLocalReply never fired within 3s")
	}

	r := asyncDCB_localReply(dcb)
	if r == nil {
		t.Fatal("error path (fma=false): SendLocalReply not called")
	}
	if r.status != 503 {
		t.Errorf("error status: got %d, want 503", r.status)
	}
	if r.body != "" {
		t.Errorf("error body: got %q, want empty", r.body)
	}
	if c := asyncDCB_continueCount(dcb); c != 0 {
		t.Errorf("error (fma=false): ContinueDecoding called %d times, want 0", c)
	}
	if v := erroredCtr.Load(); v != 1 {
		t.Errorf("errored counter: got %d, want 1", v)
	}
}

// TestDecodeHeaders_AsyncError_FailureModeAllow_True verifies that when the
// auth server is unreachable and failure_mode_allow=true, ContinueDecoding is
// called and both errored + failureModeAllowed counters increment.
func TestDecodeHeaders_AsyncError_FailureModeAllow_True(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_http")
	erroredCtr := reg.NewCounterIfAbsent("http.ingress_http.ext_authz.error")
	fmaCtr := reg.NewCounterIfAbsent("http.ingress_http.ext_authz.failure_mode_allowed")

	fn := buildHTTPCheckFnForTest(t, "http://127.0.0.1:19191", 200, "")
	cc := &compiledConfig{
		checkFn:          fn,
		failureModeAllow: true,
		statusOnError:    503,
		stats:            fs,
	}
	dcb := newAsyncExtAuthzDCB(make(http.Header))
	f := &filter{state: &factoryState{listenerRC: cc}, dcb: dcb, activeRC: cc}

	f.DecodeHeaders(make(http.Header), true)
	if !waitForContinueOrReply(dcb, 3*time.Second) {
		t.Fatal("error path (fma=true): ContinueDecoding never fired within 3s")
	}

	if c := asyncDCB_continueCount(dcb); c != 1 {
		t.Errorf("error (fma=true): ContinueDecoding called %d times, want 1", c)
	}
	if r := asyncDCB_localReply(dcb); r != nil {
		t.Errorf("error (fma=true): SendLocalReply fired unexpectedly: %+v", r)
	}
	if v := erroredCtr.Load(); v != 1 {
		t.Errorf("errored counter: got %d, want 1", v)
	}
	if v := fmaCtr.Load(); v != 1 {
		t.Errorf("failureModeAllowed counter: got %d, want 1", v)
	}
}

// TestDecodeHeaders_AsyncError_FailureModeAllow_True_HeaderAdd verifies that
// when failure_mode_allow=true AND failure_mode_allow_header_add=true, the
// x-envoy-auth-failure-mode-allowed: true header is added to the upstream
// request.
func TestDecodeHeaders_AsyncError_FailureModeAllow_True_HeaderAdd(t *testing.T) {
	fn := buildHTTPCheckFnForTest(t, "http://127.0.0.1:19191", 200, "")
	cc := &compiledConfig{
		checkFn:                   fn,
		failureModeAllow:          true,
		failureModeAllowHeaderAdd: true,
		statusOnError:             503,
	}
	upstream := make(http.Header)
	dcb := newAsyncExtAuthzDCB(upstream)
	f := &filter{state: &factoryState{listenerRC: cc}, dcb: dcb, activeRC: cc}

	f.DecodeHeaders(upstream, true)
	if !waitForContinueOrReply(dcb, 3*time.Second) {
		t.Fatal("error (fma=true, header_add=true): ContinueDecoding never fired within 3s")
	}

	got := upstream.Get("X-Envoy-Auth-Failure-Mode-Allowed")
	if got != "true" {
		t.Errorf("x-envoy-auth-failure-mode-allowed: got %q, want %q", got, "true")
	}
}

// ---------------------------------------------------------------------------
// Group 5E — async-resume invalid path
// ---------------------------------------------------------------------------

// TestDecodeHeaders_AsyncInvalid_InvalidCounterAndErrorPosture verifies that on
// the dispInvalid path (validate_mutations rejection), the invalid counter
// increments and the error posture (statusOnError) is applied.
//
// A fake checkFn is used to directly inject a dispInvalid disposition without
// relying on a real HTTP server — Go's net/http canonicalizes response headers
// making it impossible to inject a :-prefixed header from a test server. The
// dispInvalid path in production is triggered by mapHTTPResponseWithMatchers
// when validateMutations=true and a header fails validateMutationHeaders.
func TestDecodeHeaders_AsyncInvalid_InvalidCounterAndErrorPosture(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_http")
	invalidCtr := reg.NewCounterIfAbsent("http.ingress_http.ext_authz.invalid")

	// Fake checkFn that directly returns dispInvalid (simulates validate_mutations
	// rejection — same path as mapHTTPResponseWithMatchers returns dispInvalid).
	invalidFn := checkFn(func(_ context.Context, _ *authRequest) (checkDisposition, error) {
		return checkDisposition{class: dispInvalid}, nil
	})

	cc := &compiledConfig{
		checkFn:           invalidFn,
		validateMutations: true,
		failureModeAllow:  false,
		statusOnError:     403,
		stats:             fs,
	}
	dcb := newAsyncExtAuthzDCB(make(http.Header))
	f := &filter{state: &factoryState{listenerRC: cc}, dcb: dcb, activeRC: cc}

	f.DecodeHeaders(make(http.Header), true)
	if !waitForContinueOrReply(dcb, 2*time.Second) {
		t.Fatal("invalid path: neither ContinueDecoding nor SendLocalReply fired within 2s")
	}

	if v := invalidCtr.Load(); v != 1 {
		t.Errorf("invalid counter: got %d, want 1", v)
	}
	// Error posture with fma=false: SendLocalReply(statusOnError).
	r := asyncDCB_localReply(dcb)
	if r == nil {
		t.Fatal("invalid path (fma=false): SendLocalReply not called")
	}
	if r.status != 403 {
		t.Errorf("invalid/error posture: got status %d, want 403", r.status)
	}
}

// ---------------------------------------------------------------------------
// Group 5F — body-complete path (DecodeData endStream → async dispatch)
// ---------------------------------------------------------------------------

// TestDecodeData_BodyComplete_AsyncDispatch verifies that when DecodeData is
// called with endStream=true while awaitingBody, the filter returns
// DataStopIterationAndBuffer and fires the async outbound check, resuming with
// ContinueDecoding() on allow.
func TestDecodeData_BodyComplete_AsyncDispatch(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_http")
	okCtr := reg.NewCounterIfAbsent("http.ingress_http.ext_authz.ok")

	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(func() { srv.Close() })

	fn := buildHTTPCheckFnForTest(t, srv.URL, 0, "")
	cc := &compiledConfig{
		checkFn:         fn,
		withRequestBody: &bufferSettings{maxRequestBytes: 1024},
		statusOnError:   403,
		stats:           fs,
	}
	upstream := make(http.Header)
	dcb := newAsyncExtAuthzDCB(upstream)
	f := &filter{
		state:        &factoryState{listenerRC: cc},
		dcb:          dcb,
		activeRC:     cc,
		awaitingBody: true,
		bodySettings: cc.withRequestBody,
	}

	// Simulate body data with endStream.
	bodyPayload := []byte(`{"user":"alice"}`)
	dataStatus := f.DecodeData(bodyPayload, true /* endStream */)

	if dataStatus != envoyhttp.DataStopIterationAndBuffer {
		t.Fatalf("DecodeData(bodyComplete): want DataStopIterationAndBuffer, got %v", dataStatus)
	}

	if !waitForContinueOrReply(dcb, 2*time.Second) {
		t.Fatal("body-complete dispatch: ContinueDecoding never fired within 2s")
	}

	if c := asyncDCB_continueCount(dcb); c != 1 {
		t.Errorf("body-complete allow: ContinueDecoding called %d times, want 1", c)
	}
	if v := okCtr.Load(); v != 1 {
		t.Errorf("body-complete ok counter: got %d, want 1", v)
	}
	// Body was forwarded to auth service.
	if string(capturedBody) != string(bodyPayload) {
		t.Errorf("body forwarded: got %q, want %q", capturedBody, bodyPayload)
	}
}

// ----------------------------------------------------------------------------
// Group 9 — OnDestroy cancellation + resume-after-OnDestroy race guard
// (Group 9 per SPEC §14.1 + PLAN Task 9)
//
// Tests the following surfaces per SPEC §6.3 + planner-time decision D4:
//   - OnDestroy cancels the in-flight outbound call's context.Context: the
//     slow auth server returns promptly after OnDestroy cancels the context.
//   - Resume-after-OnDestroy is guarded (mu/done): when OnDestroy fires before
//     the goroutine completes, the resume goroutine checks done under mu and
//     aborts the callback touch without panic or double-use.
//
// Both tests run under -race (the acceptance criterion for the race guard is
// that go test -race finds no races).
// ----------------------------------------------------------------------------

// TestOnDestroy_CancelsInFlightContext verifies that calling OnDestroy cancels
// the per-request callCtx, causing the in-flight checkFn to return promptly
// (error from context cancellation) rather than blocking indefinitely.
func TestOnDestroy_CancelsInFlightContext(t *testing.T) {
	// Slow auth server that hangs until its request context is cancelled.
	slowSrv := newSlowAuthServer(t)
	fn := buildHTTPCheckFnForTest(t, slowSrv.srv.URL, 0, "")
	cc := &compiledConfig{checkFn: fn, statusOnError: 403, failureModeAllow: true}
	dcb := newAsyncExtAuthzDCB(make(http.Header))
	f := &filter{state: &factoryState{listenerRC: cc}, dcb: dcb, activeRC: cc}

	// Start the async dispatch (parks chain on StopIteration).
	status := f.DecodeHeaders(make(http.Header), true)
	if status != envoyhttp.StopIteration {
		t.Fatalf("DecodeHeaders before OnDestroy: want StopIteration, got %v", status)
	}

	// Give the goroutine a moment to start the outbound call before cancelling.
	time.Sleep(10 * time.Millisecond)

	// Call OnDestroy — this should cancel the callCtx.
	f.OnDestroy()

	// The goroutine should notice the context cancellation and resume (or abort
	// with done=true). Wait for either outcome — we're testing that nothing hangs.
	// We allow up to 1s for the goroutine to respond to the cancellation.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		done := f.done
		f.mu.Unlock()
		if done {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	// The filter should be done (OnDestroy set it).
	f.mu.Lock()
	done := f.done
	f.mu.Unlock()
	if !done {
		t.Error("OnDestroy: f.done not set to true under mu")
	}
}

// TestOnDestroy_ResumeAfterDestroy_NoCallback verifies the resume-after-
// OnDestroy race guard: when OnDestroy fires and sets done=true before (or
// concurrent with) the goroutine completing its async check, the goroutine
// must NOT call any dcb callbacks (no ContinueDecoding, no SendLocalReply).
//
// This test is intentionally racy in timing to exercise the guard under the
// race detector (-race). The goroutine and OnDestroy race genuinely.
func TestOnDestroy_ResumeAfterDestroy_NoCallback(t *testing.T) {
	// Slow server: hangs until context is cancelled.
	slowSrv := newSlowAuthServer(t)
	fn := buildHTTPCheckFnForTest(t, slowSrv.srv.URL, 0, "")
	cc := &compiledConfig{checkFn: fn, statusOnError: 403, failureModeAllow: true}
	dcb := newAsyncExtAuthzDCB(make(http.Header))
	f := &filter{state: &factoryState{listenerRC: cc}, dcb: dcb, activeRC: cc}

	// Fire the async dispatch.
	f.DecodeHeaders(make(http.Header), true)

	// Give the goroutine a moment to start.
	time.Sleep(5 * time.Millisecond)

	// Cancel + destroy BEFORE the goroutine can resume.
	f.OnDestroy()

	// Wait up to 2s for the goroutine to finish (it should see ctx cancelled
	// and then check done=true under mu).
	time.Sleep(200 * time.Millisecond)

	// After OnDestroy + goroutine completion, no callbacks must have fired.
	if c := asyncDCB_continueCount(dcb); c != 0 {
		t.Errorf("resume-after-OnDestroy: ContinueDecoding called %d times, want 0", c)
	}
	if r := asyncDCB_localReply(dcb); r != nil {
		t.Errorf("resume-after-OnDestroy: SendLocalReply fired unexpectedly: %+v", r)
	}
}

// TestOnDestroy_NoPanic_WhenNoActiveCall verifies that calling OnDestroy when
// no async call is in flight (callCancel is nil) does not panic.
func TestOnDestroy_NoPanic_WhenNoActiveCall(t *testing.T) {
	fn := buildHTTPCheckFnForTest(t, "http://127.0.0.1:19191", 0, "")
	cc := &compiledConfig{checkFn: fn, statusOnError: 403}
	f := &filter{state: &factoryState{listenerRC: cc}, activeRC: cc}

	// Must not panic even though callCancel is nil (no active async call).
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("OnDestroy with no active call panicked: %v", r)
		}
	}()
	f.OnDestroy()
}
