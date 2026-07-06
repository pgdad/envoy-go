package rbac

import (
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	xdsmatcherv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	rbacconfigv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	rbacv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/dynamicmetadata"
	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	rbacengine "github.com/pgdad/envoy-go/internal/rbac"
	"github.com/pgdad/envoy-go/internal/stats"
)

// mustAny packages a proto into an *anypb.Any. Mirrors phase-13/14/15 test
// helper precedent (cors / buffer / compressor / bandwidthlimit).
func mustAny(t *testing.T, msg proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

// freshFactoryCtx returns a FactoryCtx carrying a fresh Registry; used by tests
// that exercise the stat-registration path. Per ADR-0085, an empty FactoryCtx{}
// is also valid (nil Stats skips stat registration entirely).
func freshFactoryCtx() envoyhttp.FactoryCtx {
	return envoyhttp.FactoryCtx{Stats: stats.NewRegistry()}
}

// freshFactoryCtxWithRegistry returns a FactoryCtx with the supplied Registry.
// Used by Group 2 tests that need to inspect the post-Freeze idempotent
// registration path against a caller-controlled Registry (lands fully at
// Task 8 per ADR-0145; declared at Task 2 per the file-structure helper
// roster).
//
//nolint:unused // Group 8/9 stats-namespace tests consume at Task 8.
func freshFactoryCtxWithRegistry(reg *stats.Registry) envoyhttp.FactoryCtx {
	return envoyhttp.FactoryCtx{Stats: reg}
}

// allowAnyPolicy returns a minimum-viable policies map with one policy whose
// permissions + principals each contain a single `any: true` entry. The
// evaluator switch lands at Tasks 4 + 5; at Task 2 the stubs accept any
// non-empty input.
func allowAnyPolicy(name string) map[string]*rbacconfigv3.Policy {
	return map[string]*rbacconfigv3.Policy{
		name: {
			Permissions: []*rbacconfigv3.Permission{
				{Rule: &rbacconfigv3.Permission_Any{Any: true}},
			},
			Principals: []*rbacconfigv3.Principal{
				{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
			},
		},
	}
}

// happyRulesEngine returns a minimum-viable rules-engine RBAC sub-message.
func happyRulesEngine() *rbacconfigv3.RBAC {
	return &rbacconfigv3.RBAC{
		Action:   rbacconfigv3.RBAC_ALLOW,
		Policies: allowAnyPolicy("p"),
	}
}

// ----------------------------------------------------------------------------
// Group 1 — Config parse + buildCompiledConfig (per SPEC §14.1 #1 + §6.5 +
// §1.1 amendments 1-6). 17 test cases.
// ----------------------------------------------------------------------------

func TestNew_NilTC(t *testing.T) {
	factory, err := New(nil, freshFactoryCtx())
	if err == nil {
		t.Fatalf("New(nil, _): want error, got nil")
	}
	if factory != nil {
		t.Errorf("New(nil, _): want nil factory, got %v", factory)
	}
	if !strings.Contains(err.Error(), "typed_config required") {
		t.Errorf("got %q; want substring 'typed_config required'", err.Error())
	}
}

func TestNew_MalformedTC(t *testing.T) {
	bad := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff, 0xff, 0xff}}
	factory, err := New(bad, freshFactoryCtx())
	if err == nil {
		t.Fatalf("New(malformed, _): want error, got nil")
	}
	if factory != nil {
		t.Errorf("New(malformed, _): want nil factory, got %v", factory)
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("got %q; want substring 'unmarshal'", err.Error())
	}
}

func TestBuildCompiledConfig_AllSevenOuterFieldsAccepted(t *testing.T) {
	// All 7 proto-faithful outer fields per §1.1 amendment 1: rules, matcher,
	// shadow_rules, shadow_matcher, rules_stat_prefix, shadow_rules_stat_prefix,
	// track_per_rule_stats. Per amendment 2, rules + matcher are mutually
	// exclusive (rules wins); same for shadow_rules + shadow_matcher.
	c := &rbacv3.RBAC{
		Rules:                 happyRulesEngine(),
		ShadowRules:           happyRulesEngine(),
		RulesStatPrefix:       "primary",
		ShadowRulesStatPrefix: "shadow",
		TrackPerRuleStats:     true,
	}
	cc, err := buildCompiledConfig(c, freshFactoryCtx(), false /*isPerRoute*/)
	if err != nil {
		t.Fatalf("buildCompiledConfig: want success, got %v", err)
	}
	if cc.rules == nil {
		t.Error("rules engine: want non-nil")
	}
	if cc.shadowRules == nil {
		t.Error("shadow rules engine: want non-nil")
	}
	if cc.rulesStatPrefix != "primary" {
		t.Errorf("rulesStatPrefix = %q; want 'primary'", cc.rulesStatPrefix)
	}
	if cc.shadowRulesStatPrefix != "shadow" {
		t.Errorf("shadowRulesStatPrefix = %q; want 'shadow'", cc.shadowRulesStatPrefix)
	}
	if !cc.trackPerRuleStats {
		t.Error("trackPerRuleStats = false; want true")
	}
}

func TestBuildCompiledConfig_BothRulesAndMatcherSet_RulesWins(t *testing.T) {
	// Per §1.1 amendment 2 + §11.P1 + rbac.pb.go:38 proto comment: rules wins
	// when both rules + matcher are set. The matcher field is silently
	// ignored; only the rules engine is compiled.
	c := &rbacv3.RBAC{
		Rules:   happyRulesEngine(),
		Matcher: &xdsmatcherv3.Matcher{}, // would PARSE-REJECT at Task 3 if compiled
	}
	cc, err := buildCompiledConfig(c, freshFactoryCtx(), false /*isPerRoute*/)
	if err != nil {
		t.Fatalf("buildCompiledConfig: rules+matcher both set; want success (rules wins), got %v", err)
	}
	if cc.rules == nil {
		t.Error("rules engine: want non-nil")
	}
	if cc.matcher != nil {
		t.Error("matcher engine: want nil (rules wins per amendment 2)")
	}
}

func TestBuildCompiledConfig_BothShadowSet_ShadowRulesWins(t *testing.T) {
	c := &rbacv3.RBAC{
		ShadowRules:   happyRulesEngine(),
		ShadowMatcher: &xdsmatcherv3.Matcher{},
	}
	cc, err := buildCompiledConfig(c, freshFactoryCtx(), false /*isPerRoute*/)
	if err != nil {
		t.Fatalf("buildCompiledConfig: shadow_rules+shadow_matcher both set; want success, got %v", err)
	}
	if cc.shadowRules == nil {
		t.Error("shadow rules engine: want non-nil")
	}
	if cc.shadowMatcher != nil {
		t.Error("shadow matcher engine: want nil (shadow_rules wins per amendment 2)")
	}
}

func TestBuildCompiledConfig_NeitherEngineSet_WhollyInactive(t *testing.T) {
	// Per rbac.pb.go:33 "If absent, no RBAC enforcement occurs." The filter is
	// wholly inactive: rules + matcher + shadow* all nil; no PGV violation;
	// no per-stream activity.
	c := &rbacv3.RBAC{}
	cc, err := buildCompiledConfig(c, freshFactoryCtx(), false /*isPerRoute*/)
	if err != nil {
		t.Fatalf("buildCompiledConfig: empty RBAC; want success (wholly inactive), got %v", err)
	}
	if cc.rules != nil || cc.matcher != nil || cc.shadowRules != nil || cc.shadowMatcher != nil {
		t.Errorf("all engines: want nil; got rules=%v matcher=%v shadowRules=%v shadowMatcher=%v",
			cc.rules, cc.matcher, cc.shadowRules, cc.shadowMatcher)
	}
}

func TestBuildCompiledConfig_EmptyRulesStatPrefix_Accepted(t *testing.T) {
	// Per §1.1 amendment 3: empty rules_stat_prefix is permitted (proto-decode
	// default is empty string; no PGV min_len=1 requirement).
	c := &rbacv3.RBAC{
		Rules:           happyRulesEngine(),
		RulesStatPrefix: "",
	}
	cc, err := buildCompiledConfig(c, freshFactoryCtx(), false /*isPerRoute*/)
	if err != nil {
		t.Fatalf("buildCompiledConfig: empty rules_stat_prefix; want success, got %v", err)
	}
	if cc.rulesStatPrefix != "" {
		t.Errorf("rulesStatPrefix = %q; want empty", cc.rulesStatPrefix)
	}
}

func TestBuildCompiledConfig_EmptyShadowRulesStatPrefix_Accepted(t *testing.T) {
	c := &rbacv3.RBAC{
		ShadowRules:           happyRulesEngine(),
		ShadowRulesStatPrefix: "",
	}
	cc, err := buildCompiledConfig(c, freshFactoryCtx(), false /*isPerRoute*/)
	if err != nil {
		t.Fatalf("buildCompiledConfig: empty shadow_rules_stat_prefix; want success, got %v", err)
	}
	if cc.shadowRulesStatPrefix != "" {
		t.Errorf("shadowRulesStatPrefix = %q; want empty", cc.shadowRulesStatPrefix)
	}
}

func TestBuildCompiledConfig_AllThreeActionEnumValues_Accepted(t *testing.T) {
	cases := []rbacconfigv3.RBAC_Action{
		rbacconfigv3.RBAC_ALLOW,
		rbacconfigv3.RBAC_DENY,
		rbacconfigv3.RBAC_LOG,
	}
	for _, action := range cases {
		t.Run(action.String(), func(t *testing.T) {
			c := &rbacv3.RBAC{Rules: &rbacconfigv3.RBAC{
				Action:   action,
				Policies: allowAnyPolicy("p"),
			}}
			cc, err := buildCompiledConfig(c, freshFactoryCtx(), false /*isPerRoute*/)
			if err != nil {
				t.Fatalf("buildCompiledConfig(action=%v): want success, got %v", action, err)
			}
			// The action enum is captured into the (unexported) engine field;
			// that capture is pinned engine-side at internal/rbac/rbac_test.go
			// (TestBuildRulesEngine_AllThreeActionEnumValues_Accepted). The
			// consumer-relevant assertion is that all three actions are accepted
			// and produce a non-nil primary rules engine.
			if cc.rules == nil {
				t.Errorf("action=%v: cc.rules = nil; want non-nil rules engine", action)
			}
		})
	}
}

func TestBuildCompiledConfig_InvalidActionEnum_Rejected(t *testing.T) {
	// Per §1.1 amendment 4 (PGV defined_only mirror). Defensive check: any
	// action enum value not in {ALLOW=0, DENY=1, LOG=2} is rejected.
	c := &rbacv3.RBAC{Rules: &rbacconfigv3.RBAC{
		Action:   rbacconfigv3.RBAC_Action(99), // out-of-range
		Policies: allowAnyPolicy("p"),
	}}
	_, err := buildCompiledConfig(c, freshFactoryCtx(), false /*isPerRoute*/)
	if err == nil {
		t.Fatal("buildCompiledConfig: invalid action enum; want error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid action") {
		t.Errorf("got %q; want substring 'invalid action'", err.Error())
	}
}

// ----------------------------------------------------------------------------
// Group 2 — buildCompiledConfigPerRoute + parsePerRoute (per SPEC §14.1 #2 +
// §5 + §11.P1). 7 test cases.
// ----------------------------------------------------------------------------

func TestParsePerRoute_EmptyWrapper_DisabledOnRoute(t *testing.T) {
	// Per §5.1 + ADR-0125 §(xii) case (a): empty RBACPerRoute wrapper (rbac
	// field absent / nil) → disabled-on-this-route sentinel.
	any := mustAny(t, &rbacv3.RBACPerRoute{})
	msg, err := parsePerRoute(any)
	if err != nil {
		t.Fatalf("parsePerRoute(empty wrapper): want success, got %v", err)
	}
	pr, ok := msg.(*rbacv3.RBACPerRoute)
	if !ok {
		t.Fatalf("parsePerRoute returned %T; want *rbacv3.RBACPerRoute", msg)
	}
	if pr.GetRbac() != nil {
		t.Errorf("RBACPerRoute.rbac = %v; want nil (disabled-on-route)", pr.GetRbac())
	}
}

func TestParsePerRoute_RbacFieldNil_DisabledOnRoute(t *testing.T) {
	// Same as above: explicit-nil rbac field also signals disabled.
	any := mustAny(t, &rbacv3.RBACPerRoute{Rbac: nil})
	msg, err := parsePerRoute(any)
	if err != nil {
		t.Fatalf("parsePerRoute(nil rbac): want success, got %v", err)
	}
	pr, ok := msg.(*rbacv3.RBACPerRoute)
	if !ok {
		t.Fatalf("parsePerRoute returned %T; want *rbacv3.RBACPerRoute", msg)
	}
	if pr.GetRbac() != nil {
		t.Error("RBACPerRoute.rbac: want nil")
	}
}

func TestParsePerRoute_RbacFieldSet_WholesaleOverride(t *testing.T) {
	// Per §5.1 case (b): non-nil rbac field → wholesale-override (the entire
	// listener-level config is replaced for this route).
	override := &rbacv3.RBAC{
		Rules:           happyRulesEngine(),
		RulesStatPrefix: "route_override",
	}
	any := mustAny(t, &rbacv3.RBACPerRoute{Rbac: override})
	msg, err := parsePerRoute(any)
	if err != nil {
		t.Fatalf("parsePerRoute(rbac set): want success, got %v", err)
	}
	pr, ok := msg.(*rbacv3.RBACPerRoute)
	if !ok {
		t.Fatalf("parsePerRoute returned %T; want *rbacv3.RBACPerRoute", msg)
	}
	if pr.GetRbac() == nil {
		t.Fatal("RBACPerRoute.rbac: want non-nil (wholesale-override)")
	}
	if pr.GetRbac().GetRulesStatPrefix() != "route_override" {
		t.Errorf("RBACPerRoute.rbac.rules_stat_prefix = %q; want 'route_override'", pr.GetRbac().GetRulesStatPrefix())
	}
}

func TestBuildCompiledPerRoute_OverrideCarriesOwnStatPrefix_INDEPENDENT(t *testing.T) {
	// Per SPEC §5.2 + planner-time decision 17: per-route override allocates
	// its own *compiledConfig with its own filterStats, INDEPENDENT from the
	// listener namespace. ADR-0145 (lands at Task 8) ratifies; at Task 2 the
	// structural invariant is asserted via the *compiledPerRoute carrying a
	// non-nil overrideConfig whose rulesStatPrefix differs from the listener.
	reg := stats.NewRegistry()
	pr := &rbacv3.RBACPerRoute{
		Rbac: &rbacv3.RBAC{
			Rules:           happyRulesEngine(),
			RulesStatPrefix: "route_stats",
		},
	}
	cpr, err := buildCompiledPerRoute(pr, reg, "")
	if err != nil {
		t.Fatalf("buildCompiledPerRoute: %v", err)
	}
	if cpr == nil {
		t.Fatal("buildCompiledPerRoute: returned nil")
	}
	if cpr.disabled {
		t.Error("disabled = true; want false (rbac set → wholesale-override)")
	}
	if cpr.overrideConfig == nil {
		t.Fatal("overrideConfig: want non-nil")
	}
	if cpr.overrideConfig.rulesStatPrefix != "route_stats" {
		t.Errorf("rulesStatPrefix = %q; want 'route_stats'", cpr.overrideConfig.rulesStatPrefix)
	}
}

func TestParsePerRoute_MalformedAny_Rejected(t *testing.T) {
	bad := &anypb.Any{
		TypeUrl: "type.googleapis.com/envoy.extensions.filters.http.rbac.v3.RBACPerRoute",
		Value:   []byte{0xff, 0xff, 0xff, 0xff},
	}
	_, err := parsePerRoute(bad)
	if err == nil {
		t.Fatal("parsePerRoute(malformed): want error, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("got %q; want substring 'unmarshal'", err.Error())
	}
}

func TestResolvePerRouteConfig_NilMsg_FallsBackToListener(t *testing.T) {
	// Per SPEC §6.11: resolvePerRouteConfig(nil) → listenerRC (no per-route
	// → inherit listener).
	listenerCC := &compiledConfig{rulesStatPrefix: "listener"}
	state := &factoryState{
		listenerRC: listenerCC,
		perRoute:   sync.Map{},
		reg:        stats.NewRegistry(),
	}
	got, isDisabled := state.resolvePerRouteConfig(nil)
	if got != listenerCC {
		t.Errorf("resolvePerRouteConfig(nil) = %p; want listenerRC %p", got, listenerCC)
	}
	if isDisabled {
		t.Error("isDisabled = true; want false (nil msg → inherit listener)")
	}
}

func TestResolvePerRouteConfig_LazyCacheSyncMap_PointerIdentityKey(t *testing.T) {
	// Per SPEC §6.11 + ADR-0117 + ADR-0139: the per-route lazy cache is keyed
	// by *rbacv3.RBACPerRoute pointer-identity. Multi-resolve against the
	// same pointer hits a single sync.Map entry → single *compiledPerRoute
	// allocation.
	listenerCC := &compiledConfig{rulesStatPrefix: "listener"}
	state := &factoryState{
		listenerRC: listenerCC,
		perRoute:   sync.Map{},
		reg:        stats.NewRegistry(),
	}
	pr := &rbacv3.RBACPerRoute{
		Rbac: &rbacv3.RBAC{
			Rules:           happyRulesEngine(),
			RulesStatPrefix: "lazy_cached",
		},
	}
	cc1, d1 := state.resolvePerRouteConfig(pr)
	cc2, d2 := state.resolvePerRouteConfig(pr)
	cc3, d3 := state.resolvePerRouteConfig(pr)
	if cc1 == nil || cc2 == nil || cc3 == nil {
		t.Fatalf("resolvePerRouteConfig: want non-nil; got %p / %p / %p", cc1, cc2, cc3)
	}
	if d1 || d2 || d3 {
		t.Errorf("isDisabled: got (%v,%v,%v); want all false (rbac set → wholesale-override)", d1, d2, d3)
	}
	if cc1 != cc2 || cc2 != cc3 {
		t.Errorf("multi-resolve identity: got %p / %p / %p; want all same", cc1, cc2, cc3)
	}
	// Verify sync.Map entry count = 1 (single allocation).
	count := 0
	state.perRoute.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count != 1 {
		t.Errorf("sync.Map entry count = %d; want 1 (single allocation per pointer-identity)", count)
	}
}

// stubEvalContext is a test-only rbacengine.EvalContext implementation. Each
// field is a pre-populated value the per-test set-up writes; the per-method
// accessors return the field verbatim. The EvalContext interface moved to
// internal/rbac at phase-26.3; this stub satisfies it structurally and feeds
// the consumer-side evaluateEngine dispatcher tests.
type stubEvalContext struct {
	headers             map[string]string // single-value per name (canonical-cased keys)
	urlPath             string
	method              string
	destIP              net.IP
	destPort            uint32
	serverName          string
	directRemoteIP      net.IP
	remoteIP            net.IP
	downstreamPrincipal []string
}

func (s *stubEvalContext) Header(name string) (string, bool) {
	v, ok := s.headers[name]
	return v, ok
}
func (s *stubEvalContext) URLPath() string             { return s.urlPath }
func (s *stubEvalContext) Method() string              { return s.method }
func (s *stubEvalContext) DestinationIP() net.IP       { return s.destIP }
func (s *stubEvalContext) DestinationPort() uint32     { return s.destPort }
func (s *stubEvalContext) RequestedServerName() string { return s.serverName }
func (s *stubEvalContext) DirectRemoteIP() net.IP      { return s.directRemoteIP }
func (s *stubEvalContext) RemoteIP() net.IP            { return s.remoteIP }
func (s *stubEvalContext) DownstreamPrincipal() []string {
	return s.downstreamPrincipal
}
func (s *stubEvalContext) SourcedMetadata() any { return nil }
func (s *stubEvalContext) FilterState() any     { return nil }

// Compile-time assertion: *stubEvalContext satisfies rbacengine.EvalContext
// (catches interface drift during the phase-26.3 migration).
var _ rbacengine.EvalContext = (*stubEvalContext)(nil)

// ----------------------------------------------------------------------------
// Group 5 — evaluateEngine dispatcher (per SPEC §14.1 #5 + §6.9). 2 test
// cases remain. Rules-engine + matcher-engine evaluate tests moved to
// internal/rbac/rbac_test.go (Task 3); evaluateEngine (compiledConfig-shaped
// dispatcher) stays here (Task 5).
// ----------------------------------------------------------------------------

// headerEqRulesEngine builds a *rbacengine.CompiledRulesEngine with the given
// action + one policy whose single permission is header(name == value) and
// single principal is any. Built via the shared engine's public
// BuildRulesEngine constructor (the engine struct fields are unexported after
// the phase-26.3 move) with ProfileHTTP. Used by the evaluateEngine dispatcher
// tests (the dispatcher itself stays consumer-side).
func headerEqRulesEngine(t *testing.T, action rbacconfigv3.RBAC_Action, policyName, headerName, headerValue string) *rbacengine.CompiledRulesEngine {
	t.Helper()
	re, err := rbacengine.BuildRulesEngine(&rbacconfigv3.RBAC{
		Action: action,
		Policies: map[string]*rbacconfigv3.Policy{
			policyName: {
				Permissions: []*rbacconfigv3.Permission{{
					Rule: &rbacconfigv3.Permission_Header{
						Header: &routev3.HeaderMatcher{
							Name: headerName,
							HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
								StringMatch: &matcherv3.StringMatcher{
									MatchPattern: &matcherv3.StringMatcher_Exact{Exact: headerValue},
								},
							},
						},
					},
				}},
				Principals: []*rbacconfigv3.Principal{
					{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
				},
			},
		},
	}, rbacengine.ProfileHTTP)
	if err != nil {
		t.Fatalf("rbacengine.BuildRulesEngine: %v", err)
	}
	return re
}

func TestEvaluateEngine_BothPrimaryAndShadowConfigured_PrimaryDispositionWinsShadowEmitsCounter(t *testing.T) {
	// Per SPEC §6.7 + §6.9: when both primary (rules) + shadow (rules) are
	// configured, the primary disposition wins. The shadow walks but does NOT
	// affect the dispatch outcome. evaluateEngine(shadow=true) returns the
	// shadow's own result for counter emission purposes.
	primary := headerEqRulesEngine(t, rbacconfigv3.RBAC_ALLOW, "p_admin", "x-user", "admin")
	shadow := headerEqRulesEngine(t, rbacconfigv3.RBAC_DENY, "p_admin", "x-user", "admin")
	cc := &compiledConfig{rules: primary, shadowRules: shadow}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "admin"}}
	primaryResult, primaryName := evaluateEngine(cc, ctx, false /*shadow*/)
	shadowResult, shadowName := evaluateEngine(cc, ctx, true /*shadow*/)
	if primaryResult != rbacengine.Allowed {
		t.Errorf("primary: got %v, want rbacengine.Allowed (ALLOW + match)", primaryResult)
	}
	if primaryName != "p_admin" {
		t.Errorf("primary name: got %q, want %q", primaryName, "p_admin")
	}
	if shadowResult != rbacengine.Denied {
		t.Errorf("shadow: got %v, want rbacengine.Denied (DENY + match — independent of primary)", shadowResult)
	}
	if shadowName != "p_admin" {
		t.Errorf("shadow name: got %q, want %q", shadowName, "p_admin")
	}
}

func TestEvaluateEngine_BothEnginesUnset_DefensiveAllowed(t *testing.T) {
	// Per SPEC §6.9: defensive ALLOWED when both engines (rules + matcher) are
	// nil. The DecodeHeaders body's "both engines unset" fast-path triggers
	// passthrough BEFORE evaluateEngine is called in production; but the
	// evaluateEngine helper itself returns rbacengine.Allowed defensively if
	// called with neither set (e.g., shadow=true when no shadow configured).
	cc := &compiledConfig{}
	ctx := &stubEvalContext{}
	result, name := evaluateEngine(cc, ctx, false /*shadow*/)
	if result != rbacengine.Allowed {
		t.Errorf("result: got %v, want rbacengine.Allowed (defensive)", result)
	}
	if name != "" {
		t.Errorf("name: got %q, want \"\"", name)
	}
	// Shadow path with no shadow configured — same defensive ALLOWED.
	result, name = evaluateEngine(cc, ctx, true /*shadow*/)
	if result != rbacengine.Allowed {
		t.Errorf("shadow result: got %v, want rbacengine.Allowed (defensive)", result)
	}
	if name != "" {
		t.Errorf("shadow name: got %q, want \"\"", name)
	}
}

// ----------------------------------------------------------------------------
// Group 6 — DecodeHeaders gating + SendLocalReply (per SPEC §14.1 #6 + §6.7).
// 9 test cases.
//
// Each test wires a *filter to a rbacFakeCB DecoderFilterCallbacks, invokes
// DecodeHeaders against a synthetic http.Header, and asserts (a) the returned
// FilterHeadersStatus + (b) SendLocalReply invocation arguments (or absence)
// + (c) per-route resolution path + (d) counter-side STUB invocation.
// ----------------------------------------------------------------------------

// rbacLocalReplyArgs captures one SendLocalReply invocation.
type rbacLocalReplyArgs struct {
	status  int
	body    string
	headers envoyhttp.OrderedHeaders
}

// rbacFakeCB is the test-double DecoderFilterCallbacks for Groups 6+8.
// RequestRouteConfig returns the settable routeCfg (default nil → listener
// inherit per resolvePerRouteConfig); SendLocalReply records its invocation
// args for assertion; DownstreamPrincipal returns the settable principals
// slice (default nil for plaintext per Group 7 precedent).
type rbacFakeCB struct {
	mu         sync.Mutex
	routeCfg   proto.Message       // returned by RequestRouteConfig; nil → inherit listener
	principals []string            // returned by DownstreamPrincipal; nil for plaintext
	localReply *rbacLocalReplyArgs // captured at SendLocalReply; nil if never called
}

func (c *rbacFakeCB) ContinueDecoding() {}
func (c *rbacFakeCB) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.localReply = &rbacLocalReplyArgs{status: status, body: body, headers: headers}
}
func (c *rbacFakeCB) RequestRouteConfig() proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.routeCfg
}
func (c *rbacFakeCB) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (c *rbacFakeCB) EncodeHeaders(http.Header, bool) {}
func (c *rbacFakeCB) EncodeData([]byte, bool)         {}
func (c *rbacFakeCB) EncodeTrailers(http.Header)      {}
func (c *rbacFakeCB) DownstreamPrincipal() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.principals
}

// ADR-0165 callback-surface extension stubs (phase-18.2 Task 4).
func (c *rbacFakeCB) DownstreamRemoteAddr() net.Addr   { return nil }
func (c *rbacFakeCB) DownstreamLocalAddr() net.Addr    { return nil }
func (c *rbacFakeCB) DownstreamTLSServerName() string  { return "" }
func (c *rbacFakeCB) DownstreamTLSPeerCertDER() []byte { return nil }
func (c *rbacFakeCB) DownstreamProtocol() string       { return "" }
func (c *rbacFakeCB) ListenerPrincipal() string        { return "" }

// ADR-0192 callback-surface extension stubs (phase-22.2 Task 5).
func (c *rbacFakeCB) DownstreamTLSConnectionState() *tls.ConnectionState { return nil }
func (c *rbacFakeCB) DynamicMetadata() *dynamicmetadata.Bucket           { return nil }

// ADR-0198 callback-surface extension stubs (phase-24.1 Task 5 — DELTA-2).
func (c *rbacFakeCB) RouteRateLimits() []*routev3.RateLimit       { return nil }
func (c *rbacFakeCB) VirtualHostRateLimits() []*routev3.RateLimit { return nil }
func (c *rbacFakeCB) RouteMetadata() *corev3.Metadata             { return nil }
func (c *rbacFakeCB) RouteIncludeVhRateLimits() bool              { return false }

// newFilterWithRBAC constructs a *filter wrapping the supplied *rbacv3.RBAC
// listener-level proto + freshly attached rbacFakeCB. Used by Group 6 +
// Group 8 tests.
func newFilterWithRBAC(t *testing.T, listener *rbacv3.RBAC) (*filter, *rbacFakeCB) {
	t.Helper()
	factory, err := New(mustAny(t, listener), freshFactoryCtx())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inst := factory()
	fl, ok := inst.Decoder.(*filter)
	if !ok {
		t.Fatalf("Decoder: want *filter; got %T", inst.Decoder)
	}
	cb := &rbacFakeCB{}
	fl.SetDecoderCallbacks(cb)
	return fl, cb
}

// allowAdminRBAC returns a listener-level RBAC proto: action=ALLOW, one
// policy that matches when X-User == "admin".
func allowAdminRBAC() *rbacv3.RBAC {
	return &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action: rbacconfigv3.RBAC_ALLOW,
			Policies: map[string]*rbacconfigv3.Policy{
				"p_admin": {
					Permissions: []*rbacconfigv3.Permission{{
						Rule: &rbacconfigv3.Permission_Header{
							Header: &routev3.HeaderMatcher{
								Name: "x-user",
								HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
									StringMatch: &matcherv3.StringMatcher{
										MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "admin"},
									},
								},
							},
						},
					}},
					Principals: []*rbacconfigv3.Principal{
						{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
					},
				},
			},
		},
	}
}

func TestDecodeHeaders_ListenerLevelAllowMatch_HeaderContinue(t *testing.T) {
	// Per SPEC §6.7: listener-level ALLOW + match → HeaderContinue (no
	// SendLocalReply; passthrough to next filter).
	fl, cb := newFilterWithRBAC(t, allowAdminRBAC())
	hdr := http.Header{"X-User": []string{"admin"}}
	status := fl.DecodeHeaders(hdr, false)
	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (ALLOW + match)", status)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on allow path; got %+v", cb.localReply)
	}
}

func TestDecodeHeaders_ListenerLevelDenyMatch_HeaderStopIteration_SendLocalReply403(t *testing.T) {
	// Per SPEC §6.7 + §1.1 amendment 10: ALLOW + no-match → DENY disposition →
	// SendLocalReply(403, "RBAC: access denied", ...) + StopIteration.
	fl, cb := newFilterWithRBAC(t, allowAdminRBAC())
	hdr := http.Header{"X-User": []string{"guest"}}
	status := fl.DecodeHeaders(hdr, false)
	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration (DENY)", status)
	}
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want invocation on DENY; got nil")
	}
	if cb.localReply.status != 403 {
		t.Errorf("SendLocalReply status: got %d, want 403", cb.localReply.status)
	}
}

func TestDecodeHeaders_SendLocalReply_Body19Bytes_RBACAccessDenied(t *testing.T) {
	// Per §1.1 amendment 10 + §11.P5: deny body byte-exact "RBAC: access denied"
	// (19 bytes ASCII; no trailing newline).
	fl, cb := newFilterWithRBAC(t, allowAdminRBAC())
	hdr := http.Header{"X-User": []string{"guest"}}
	fl.DecodeHeaders(hdr, false)
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want invocation on DENY; got nil")
	}
	if cb.localReply.body != "RBAC: access denied" {
		t.Errorf("body: got %q, want %q", cb.localReply.body, "RBAC: access denied")
	}
	if got := len(cb.localReply.body); got != 19 {
		t.Errorf("body length: got %d, want 19 (19-byte ASCII per amendment 10)", got)
	}
}

func TestDecodeHeaders_SendLocalReply_4HeaderSet_LowercaseWireForm(t *testing.T) {
	// Per §1.1 amendment 10 + §11.P5: the filter passes Content-Type: text/plain
	// in the OrderedHeaders carrier; the framework's local-reply path injects
	// content-length (19) + date + server (lowercase wire-form). At the filter
	// boundary we assert ONLY the filter-supplied entry — the framework injects
	// the remaining three at write-time. Per ADR-0140.
	fl, cb := newFilterWithRBAC(t, allowAdminRBAC())
	hdr := http.Header{"X-User": []string{"guest"}}
	fl.DecodeHeaders(hdr, false)
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want invocation on DENY; got nil")
	}
	got := cb.localReply.headers.Get("Content-Type")
	if got != "text/plain" {
		t.Errorf("Content-Type: got %q, want %q", got, "text/plain")
	}
	if len(cb.localReply.headers) != 1 {
		t.Errorf("filter-supplied header count: got %d, want 1 (Content-Type only; framework injects the other 3)", len(cb.localReply.headers))
	}
}

func TestDecodeHeaders_SendLocalReply_KeepAliveDisposition_NoConnectionClose(t *testing.T) {
	// Per §1.1 amendment 10: the deny-decision fires BEFORE body, so there is
	// no partial-body-consumption ambiguity; the connection stays keep-alive
	// (NO `connection: close` header). The filter MUST NOT inject a Connection
	// header on the SendLocalReply path.
	fl, cb := newFilterWithRBAC(t, allowAdminRBAC())
	hdr := http.Header{"X-User": []string{"guest"}}
	fl.DecodeHeaders(hdr, false)
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want invocation on DENY; got nil")
	}
	if got := cb.localReply.headers.Get("Connection"); got != "" {
		t.Errorf("Connection header: got %q, want \"\" (keep-alive; no connection-close)", got)
	}
}

func TestDecodeHeaders_LOGMatch_HeaderContinue_AllowedCounterIncremented(t *testing.T) {
	// Per §1.1 amendment 5 + 8: LOG-matched requests result in HeaderContinue
	// (LOG always-allows) AND the allowed counter increments (NOT a separate
	// `logged` counter per amendment 8 — no logged counter exists in Envoy
	// v1.37.2). emitPrimaryCounters STUB at Task 7 increments the base allowed
	// counter; full per-policy emission lands at Task 9.
	listener := &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action: rbacconfigv3.RBAC_LOG,
			Policies: map[string]*rbacconfigv3.Policy{
				"p_log": {
					Permissions: []*rbacconfigv3.Permission{{
						Rule: &rbacconfigv3.Permission_Header{
							Header: &routev3.HeaderMatcher{
								Name: "x-user",
								HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
									StringMatch: &matcherv3.StringMatcher{
										MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "admin"},
									},
								},
							},
						},
					}},
					Principals: []*rbacconfigv3.Principal{
						{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
					},
				},
			},
		},
	}
	fl, cb := newFilterWithRBAC(t, listener)
	if fl.state.listenerRC.stats == nil {
		t.Fatal("listenerRC.stats: want non-nil (ctx.Stats was non-nil)")
	}
	before := fl.state.listenerRC.stats.allowed.Load()
	hdr := http.Header{"X-User": []string{"admin"}}
	status := fl.DecodeHeaders(hdr, false)
	after := fl.state.listenerRC.stats.allowed.Load()
	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (LOG always-allows)", status)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on LOG-matched path; got %+v", cb.localReply)
	}
	if delta := after - before; delta != 1 {
		t.Errorf("allowed counter delta: got %d, want 1 (LOG folds into allowed per amendment 8)", delta)
	}
}

func TestDecodeHeaders_PerRouteDisabled_PassthroughNoCounters(t *testing.T) {
	// Per §5.1 + ADR-0125 §(xii) case (a): empty RBACPerRoute wrapper (rbac
	// field nil) → wholly disabled on this route. DecodeHeaders sets
	// f.passthrough = true; returns HeaderContinue; NO counters increment.
	fl, cb := newFilterWithRBAC(t, allowAdminRBAC())
	// Wire per-route TPFC = empty wrapper (rbac field nil → disabled).
	cb.routeCfg = &rbacv3.RBACPerRoute{}
	before := fl.state.listenerRC.stats.allowed.Load()
	beforeDenied := fl.state.listenerRC.stats.denied.Load()
	// Request that WOULD deny under listener (X-User=guest with ALLOW+admin).
	hdr := http.Header{"X-User": []string{"guest"}}
	status := fl.DecodeHeaders(hdr, false)
	after := fl.state.listenerRC.stats.allowed.Load()
	afterDenied := fl.state.listenerRC.stats.denied.Load()
	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (per-route disabled)", status)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on per-route-disabled path; got %+v", cb.localReply)
	}
	if !fl.passthrough {
		t.Error("passthrough: want true on per-route-disabled")
	}
	if after != before {
		t.Errorf("allowed delta: got %d, want 0 (no counters on per-route-disabled)", after-before)
	}
	if afterDenied != beforeDenied {
		t.Errorf("denied delta: got %d, want 0 (no counters on per-route-disabled)", afterDenied-beforeDenied)
	}
}

func TestDecodeHeaders_PerRouteOverride_INDEPENDENTCounterNamespace(t *testing.T) {
	// Per §5.2 + ADR-0145: per-route wholesale-override allocates its own
	// *compiledConfig with its own filterStats; counters route under the
	// override's stat_prefix namespace; listener-level counters are UNCHANGED.
	// At Task 7 the structural per-route resolution is asserted; the
	// stat-name-pinned INDEPENDENT-namespace assertion is STUBBED — at Task 8
	// (ADR-0145 finalized) the namespace-canonicalization is byte-pinned.
	fl, cb := newFilterWithRBAC(t, allowAdminRBAC())
	// Wire per-route TPFC carrying its own RBAC override (DENY guests; stats
	// allocated under "override" prefix).
	cb.routeCfg = &rbacv3.RBACPerRoute{
		Rbac: &rbacv3.RBAC{
			RulesStatPrefix: "override",
			Rules: &rbacconfigv3.RBAC{
				Action: rbacconfigv3.RBAC_DENY,
				Policies: map[string]*rbacconfigv3.Policy{
					"p_guest": {
						Permissions: []*rbacconfigv3.Permission{{
							Rule: &rbacconfigv3.Permission_Header{
								Header: &routev3.HeaderMatcher{
									Name: "x-user",
									HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
										StringMatch: &matcherv3.StringMatcher{
											MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "guest"},
										},
									},
								},
							},
						}},
						Principals: []*rbacconfigv3.Principal{
							{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
						},
					},
				},
			},
		},
	}
	beforeListenerAllowed := fl.state.listenerRC.stats.allowed.Load()
	beforeListenerDenied := fl.state.listenerRC.stats.denied.Load()
	hdr := http.Header{"X-User": []string{"guest"}}
	status := fl.DecodeHeaders(hdr, false)
	afterListenerAllowed := fl.state.listenerRC.stats.allowed.Load()
	afterListenerDenied := fl.state.listenerRC.stats.denied.Load()
	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration (per-route DENY + match)", status)
	}
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want invocation (per-route DENY + match); got nil")
	}
	if cb.localReply.status != 403 {
		t.Errorf("SendLocalReply status: got %d, want 403", cb.localReply.status)
	}
	// Listener-level counters UNCHANGED: per-route INDEPENDENT-stats.
	if afterListenerAllowed != beforeListenerAllowed {
		t.Errorf("listener allowed delta: got %d, want 0 (per-route INDEPENDENT)", afterListenerAllowed-beforeListenerAllowed)
	}
	if afterListenerDenied != beforeListenerDenied {
		t.Errorf("listener denied delta: got %d, want 0 (per-route INDEPENDENT)", afterListenerDenied-beforeListenerDenied)
	}
	// Verify the per-route override config was resolved + activeRC pinned.
	if fl.activeRC == nil {
		t.Fatal("activeRC: want non-nil (per-route override resolved)")
	}
	if fl.activeRC == fl.state.listenerRC {
		t.Error("activeRC: want NOT equal to listenerRC (per-route INDEPENDENT *compiledConfig)")
	}
	if fl.activeRC.rulesStatPrefix != "override" {
		t.Errorf("activeRC.rulesStatPrefix: got %q, want %q", fl.activeRC.rulesStatPrefix, "override")
	}
}

func TestDecodeHeaders_BothEnginesUnset_PassthroughNoCounters(t *testing.T) {
	// Per rbac.pb.go:33 + SPEC §6.7: both primary engines (rules + matcher)
	// unset → filter wholly inactive; passthrough; NO counter activity.
	// listener-level RBAC with neither rules nor matcher set.
	listener := &rbacv3.RBAC{}
	fl, cb := newFilterWithRBAC(t, listener)
	// stats should be allocated (since ctx.Stats was non-nil) — the gate is the
	// "both engines unset" pre-counter early-return, not the absence of stats.
	hdr := http.Header{"X-User": []string{"anyone"}}
	status := fl.DecodeHeaders(hdr, false)
	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (both engines unset)", status)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on both-engines-unset path; got %+v", cb.localReply)
	}
	if !fl.passthrough {
		t.Error("passthrough: want true on both-engines-unset")
	}
	if fl.state.listenerRC.stats == nil {
		t.Skip("listenerRC.stats nil — skipping counter delta assertion")
	}
	if got := fl.state.listenerRC.stats.allowed.Load(); got != 0 {
		t.Errorf("allowed counter: got %d, want 0 (no counters on both-engines-unset)", got)
	}
	if got := fl.state.listenerRC.stats.denied.Load(); got != 0 {
		t.Errorf("denied counter: got %d, want 0", got)
	}
}

// Group 8 tests (matcher-engine framework primitive integration) moved to
// internal/rbac/rbac_test.go (Task 3).

// ----------------------------------------------------------------------------
// Group 9 — Stats namespace integration (per PLAN line 66 + SPEC §14.1 #9 +
// ADR-0145 + §1.1 amendment 9 + §11.P7). 6 test cases:
//
//   - TestStatsNamespace_AllFourBaseCountersRegistered — verifies the 4 base
//     counters (allowed/denied/shadow_allowed/shadow_denied) land on the
//     Registry per active prefix combination.
//   - TestStatsNamespace_SN2Reuse_NoNewSN10Rule — verifies the registered
//     counter names follow the SN2-compatible `http.*` hierarchical shape so
//     the existing default-branch flatten in internal/stats/name.go covers
//     them with NO new SN10 rule.
//   - TestStatsNamespace_HCMRootedPath_HttpHCMRbacPrefixCounter — verifies the
//     full `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>` shape
//     was assembled correctly (empirically RATIFIED at Task 8 via Envoy
//     v1.37.2 scrape).
//   - TestStatsNamespace_PerPolicyLazyAllocation_OnFirstMatch — verifies the
//     (*rbacengine.PerPolicyCounters).Inc lazy-cache contract (sync.Map LoadOrStore +
//     NewCounterIfAbsent post-Freeze idempotent).
//   - TestStatsNamespace_NewFilterStatsIfAbsent_Idempotent — verifies two
//     resolve passes against the same (hcmPrefix, primaryPrefix, shadowPrefix)
//     triple yield pointer-identical *stats.Counter instances.
//   - TestStatsNamespace_PerRouteINDEPENDENT_ListenerCountersUnaffected —
//     CF-Task7-M5 pin: distinct per-route prefix (`override`) emits to a
//     SEPARATE counter; listener-level counters DON'T touch the per-route
//     counter values.
// ----------------------------------------------------------------------------

// collectMetricNames returns the Registry's full registered-name set via Walk.
// Used by Group 9 tests to assert the counter-name shape.
func collectMetricNames(reg *stats.Registry) []string {
	var names []string
	reg.Walk(func(m stats.Metric) {
		names = append(names, m.Name())
	})
	return names
}

// containsString reports whether haystack contains needle (exact match).
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestStatsNamespace_AllFourBaseCountersRegistered(t *testing.T) {
	// Per ADR-0145 §Decision (vi): all 4 base counters are registered
	// unconditionally (predeclared empty counters for scrape stability —
	// matches Prometheus best practice). Verify the 4 names land on the
	// Registry post-buildCompiledConfig.
	reg := stats.NewRegistry()
	c := &rbacv3.RBAC{
		Rules:                 happyRulesEngine(),
		ShadowRules:           happyRulesEngine(),
		RulesStatPrefix:       "primary_rules",
		ShadowRulesStatPrefix: "shadow_rules",
	}
	ctx := envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "hcm_test"}
	cc, err := buildCompiledConfig(c, ctx, false /*isPerRoute*/)
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	if cc.stats == nil {
		t.Fatal("cc.stats: want non-nil")
	}
	names := collectMetricNames(reg)
	want := []string{
		"http.hcm_test.rbac.primary_rules.allowed",
		"http.hcm_test.rbac.primary_rules.denied",
		"http.hcm_test.rbac.shadow_rules.shadow_allowed",
		"http.hcm_test.rbac.shadow_rules.shadow_denied",
	}
	for _, w := range want {
		if !containsString(names, w) {
			t.Errorf("missing counter %q in Registry; got names=%v", w, names)
		}
	}
}

func TestStatsNamespace_SN2Reuse_NoNewSN10Rule(t *testing.T) {
	// Per §1.1 amendment 9 + §11.P7 + ADR-0145: the registered counter names
	// follow the SN2-reuse `http.<HCM>.<rest>` hierarchical shape so the
	// existing default-branch flatten in internal/stats/name.go's
	// flattenToProm covers them with NO new SN10 rule. This test pins the
	// name shape (the actual Prometheus rendering is exercised at fixture
	// 0018 + internal/stats/name_test.go's existing SN2 tests).
	reg := stats.NewRegistry()
	c := &rbacv3.RBAC{
		Rules:           happyRulesEngine(),
		RulesStatPrefix: "myprefix",
	}
	ctx := envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "hcm_test"}
	_, err := buildCompiledConfig(c, ctx, false /*isPerRoute*/)
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	names := collectMetricNames(reg)
	for _, n := range names {
		if !strings.HasPrefix(n, "http.hcm_test.rbac.") {
			t.Errorf("counter %q does NOT have SN2-compatible prefix `http.hcm_test.rbac.` — SN2-reuse violated", n)
		}
		// Defensive: ensure no leading dot / double dot / trailing dot (which
		// would fail the Registry nameRE).
		if strings.Contains(n, "..") {
			t.Errorf("counter %q contains double-dot — would fail nameRE", n)
		}
	}
}

func TestStatsNamespace_HCMRootedPath_HttpHCMRbacPrefixCounter(t *testing.T) {
	// Per ADR-0145 + §1.1 amendment 9 (RATIFIED via Task 8 empirical scrape
	// against reference Envoy v1.37.2): the full counter-name shape is
	// `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>`. This test
	// pins the exact name for one counter (allowed) under a known prefix
	// combination.
	reg := stats.NewRegistry()
	c := &rbacv3.RBAC{
		Rules:           happyRulesEngine(),
		RulesStatPrefix: "myrules",
	}
	ctx := envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "ingress_http"}
	cc, err := buildCompiledConfig(c, ctx, false /*isPerRoute*/)
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	if cc.stats == nil || cc.stats.allowed == nil {
		t.Fatal("cc.stats.allowed: want non-nil")
	}
	want := "http.ingress_http.rbac.myrules.allowed"
	if got := cc.stats.allowed.Name(); got != want {
		t.Errorf("allowed counter name: got %q, want %q (HCM-rooted SN2-reuse per ADR-0145)", got, want)
	}
	// Also verify the empty rules_stat_prefix path falls back to "rbac" per
	// §1.1 amendment 3 + ADR-0145 §namespacePrefix discipline.
	reg2 := stats.NewRegistry()
	c2 := &rbacv3.RBAC{Rules: happyRulesEngine()}
	cc2, err := buildCompiledConfig(c2, envoyhttp.FactoryCtx{Stats: reg2, StatPrefix: "hcm2"}, false)
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	wantFallback := "http.hcm2.rbac.rbac.allowed"
	if got := cc2.stats.allowed.Name(); got != wantFallback {
		t.Errorf("empty rules_stat_prefix fallback: got %q, want %q", got, wantFallback)
	}
}

func TestStatsNamespace_PerPolicyLazyAllocation_OnFirstMatch(t *testing.T) {
	// Per ADR-0145 §Decision (iii) + §11.P10: per-policy counters are
	// lazy-allocated on first match via filterStats.perPolicy.Inc → sync.Map
	// LoadOrStore + NewCounterIfAbsent. The counter name shape per the Task 8
	// empirical scrape is `<base_prefix>.policy.<policy_name>.<suffix>`
	// (REFINES the SPEC line 1842 hypothesis which omitted the `.policy.`
	// segment infix — SPEC stat-table amends at Task 9 alongside ADR-0146).
	//
	// The per-policy lazy-cache type moved to internal/rbac at phase-26.3
	// (PerPolicyCounters, D-26.3-7); the sync.Map entry-count contract is pinned
	// in internal/rbac/perpolicy_test.go. This consumer test pins the
	// operator-visible Registry surface: lazy allocation (no counter pre-
	// emission) + idempotent registration (a single Registry counter after two
	// increments, value = 2) through the consumer's base-prefix wiring.
	reg := stats.NewRegistry()
	c := &rbacv3.RBAC{
		Rules:             happyRulesEngine(),
		RulesStatPrefix:   "rp",
		TrackPerRuleStats: true,
	}
	ctx := envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "hcm"}
	cc, err := buildCompiledConfig(c, ctx, false /*isPerRoute*/)
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}
	if cc.stats == nil {
		t.Fatal("cc.stats: want non-nil")
	}
	// Pre-emission: no per-policy counter registered (lazy).
	want := "http.hcm.rbac.rp.policy.p_admin.allowed"
	if containsString(collectMetricNames(reg), want) {
		t.Errorf("per-policy counter %q present before any emission (want lazy)", want)
	}
	// First emission: allocates + increments via the consumer's base prefix.
	cc.stats.perPolicy.Inc(cc.stats.reg, cc.stats.primaryBase, "p_admin", "allowed")
	if !containsString(collectMetricNames(reg), want) {
		t.Errorf("missing per-policy counter %q after first Inc", want)
	}
	// Second emission: idempotent registration (no duplicate Registry entry).
	cc.stats.perPolicy.Inc(cc.stats.reg, cc.stats.primaryBase, "p_admin", "allowed")
	regCount := 0
	for _, n := range collectMetricNames(reg) {
		if n == want {
			regCount++
		}
	}
	if regCount != 1 {
		t.Errorf("Registry entries for %q after 2 emissions: got %d, want 1 (idempotent)", want, regCount)
	}
	// Verify both increments landed (counter value = 2).
	var found *stats.Counter
	reg.Walk(func(m stats.Metric) {
		if m.Name() == want {
			if c, ok := m.(*stats.Counter); ok {
				found = c
			}
		}
	})
	if found == nil {
		t.Fatalf("per-policy counter %q not on Registry", want)
	}
	if got := found.Load(); got != 2 {
		t.Errorf("per-policy counter value: got %d, want 2", got)
	}
}

func TestStatsNamespace_NewFilterStatsIfAbsent_Idempotent(t *testing.T) {
	// Per ADR-0117 + ADR-0139 + ADR-0145 §Decision (v): newFilterStatsIfAbsent
	// is post-Freeze idempotent. Two resolve passes against the same
	// (hcmPrefix, primaryPrefix, shadowPrefix) triple yield pointer-identical
	// *stats.Counter instances. The Registry can be Frozen between the two
	// calls — the second call must succeed (NOT panic).
	reg := stats.NewRegistry()
	fs1 := newFilterStatsIfAbsent(reg, "hcm_x", "primary", "shadow")
	reg.Freeze()
	fs2 := newFilterStatsIfAbsent(reg, "hcm_x", "primary", "shadow")
	if fs1.allowed != fs2.allowed {
		t.Errorf("allowed counter pointer-identity: fs1=%p, fs2=%p (NewCounterIfAbsent must return same instance)", fs1.allowed, fs2.allowed)
	}
	if fs1.denied != fs2.denied {
		t.Errorf("denied counter pointer-identity: fs1=%p, fs2=%p", fs1.denied, fs2.denied)
	}
	if fs1.shadowAllowed != fs2.shadowAllowed {
		t.Errorf("shadowAllowed counter pointer-identity: fs1=%p, fs2=%p", fs1.shadowAllowed, fs2.shadowAllowed)
	}
	if fs1.shadowDenied != fs2.shadowDenied {
		t.Errorf("shadowDenied counter pointer-identity: fs1=%p, fs2=%p", fs1.shadowDenied, fs2.shadowDenied)
	}
	// CF-Task2-I3 + CF-Task7-M6 fix: newFilterStats also goes through
	// NewCounterIfAbsent so multiple listener-level filters sharing the same
	// (empty) prefix do NOT panic on duplicate registration.
	reg2 := stats.NewRegistry()
	fs3 := newFilterStats(reg2, "hcm_y", "", "")
	fs4 := newFilterStats(reg2, "hcm_y", "", "") // 2nd call with same (empty,empty) — MUST NOT panic
	if fs3.allowed != fs4.allowed {
		t.Errorf("listener-level duplicate registration: fs3=%p, fs4=%p (must be idempotent per CF-Task7-M6)", fs3.allowed, fs4.allowed)
	}
}

func TestStatsNamespace_PerRouteINDEPENDENT_ListenerCountersUnaffected(t *testing.T) {
	// CF-Task7-M5 pin: distinct per-route rules_stat_prefix ("override" vs
	// listener's "default") MUST emit to a separate counter namespace. Per
	// ADR-0145 §Decision (iv) — per-route INDEPENDENT-stats via
	// newFilterStatsIfAbsent. Listener-level counters' values DO NOT track
	// per-route counter increments.
	//
	// Listener: ALLOW + match admin (rules_stat_prefix=default).
	// Per-route: DENY + match guest (rules_stat_prefix=override).
	// Send X-User=guest:
	//   - Listener engine (if walked) would DENY (no admin match).
	//   - Per-route engine DENIES on guest match.
	// Per-route INDEPENDENT-stats: only the per-route `override.denied`
	// counter increments; listener `default.denied` STAYS AT ZERO.
	listenerProto := &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action: rbacconfigv3.RBAC_ALLOW,
			Policies: map[string]*rbacconfigv3.Policy{
				"p_admin": {
					Permissions: []*rbacconfigv3.Permission{{
						Rule: &rbacconfigv3.Permission_Header{
							Header: &routev3.HeaderMatcher{
								Name: "x-user",
								HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
									StringMatch: &matcherv3.StringMatcher{
										MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "admin"},
									},
								},
							},
						},
					}},
					Principals: []*rbacconfigv3.Principal{{Identifier: &rbacconfigv3.Principal_Any{Any: true}}},
				},
			},
		},
		RulesStatPrefix: "default",
	}
	reg := stats.NewRegistry()
	factory, err := New(mustAny(t, listenerProto), envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "hcm_pin"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inst := factory()
	fl, ok := inst.Decoder.(*filter)
	if !ok {
		t.Fatalf("Decoder: want *filter, got %T", inst.Decoder)
	}
	cb := &rbacFakeCB{}
	fl.SetDecoderCallbacks(cb)
	cb.routeCfg = &rbacv3.RBACPerRoute{
		Rbac: &rbacv3.RBAC{
			RulesStatPrefix: "override",
			Rules: &rbacconfigv3.RBAC{
				Action: rbacconfigv3.RBAC_DENY,
				Policies: map[string]*rbacconfigv3.Policy{
					"p_guest": {
						Permissions: []*rbacconfigv3.Permission{{
							Rule: &rbacconfigv3.Permission_Header{
								Header: &routev3.HeaderMatcher{
									Name: "x-user",
									HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
										StringMatch: &matcherv3.StringMatcher{
											MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "guest"},
										},
									},
								},
							},
						}},
						Principals: []*rbacconfigv3.Principal{{Identifier: &rbacconfigv3.Principal_Any{Any: true}}},
					},
				},
			},
		},
	}
	// Capture listener counter pointers BEFORE the per-route resolve (the
	// listener counters are allocated at New time).
	listenerDenied := fl.state.listenerRC.stats.denied
	listenerAllowed := fl.state.listenerRC.stats.allowed
	if listenerDenied.Name() != "http.hcm_pin.rbac.default.denied" {
		t.Errorf("listener denied counter name: got %q, want %q", listenerDenied.Name(), "http.hcm_pin.rbac.default.denied")
	}
	hdr := http.Header{"X-User": []string{"guest"}}
	fl.DecodeHeaders(hdr, false)
	// Listener counters MUST stay at zero (per-route engine handled this req).
	if v := listenerAllowed.Load(); v != 0 {
		t.Errorf("listener allowed: got %d, want 0 (per-route INDEPENDENT)", v)
	}
	if v := listenerDenied.Load(); v != 0 {
		t.Errorf("listener denied: got %d, want 0 (per-route INDEPENDENT)", v)
	}
	// Per-route counter MUST increment (DENY + match guest).
	if fl.activeRC == nil || fl.activeRC.stats == nil || fl.activeRC.stats.denied == nil {
		t.Fatal("per-route activeRC.stats.denied: want non-nil after resolve")
	}
	if v := fl.activeRC.stats.denied.Load(); v != 1 {
		t.Errorf("per-route denied: got %d, want 1", v)
	}
	if got, want := fl.activeRC.stats.denied.Name(), "http.hcm_pin.rbac.override.denied"; got != want {
		t.Errorf("per-route denied counter name: got %q, want %q", got, want)
	}
	// Verify pointer-distinct *stats.Counter instances.
	if listenerDenied == fl.activeRC.stats.denied {
		t.Error("listener.denied and per-route.denied counters share pointer-identity; INDEPENDENT violation")
	}
}

// ----------------------------------------------------------------------------
// Group 9 (extension) — Task 9 / ADR-0146 finalization: shadow + LOG-partial +
// track_per_rule_stats per-policy emission discipline.
//
// At Task 8 the lazy-allocation contract for per-policy counters landed via
// (*rbacengine.PerPolicyCounters).Inc (sync.Map LoadOrStore + NewCounterIfAbsent); the
// emit*Counters call-sites were STUBs that incremented base counters only.
// Task 9 wires per-policy emission into emit*Counters (gated by
// trackPerRuleStats) + finalizes the shadow-path orchestration per ADR-0146.
//
// The following test cases pin:
//   - LOG-partial + track_per_rule_stats → per-policy `.allowed` increments
//     (LOG folds into allowed per amendment 8; matched-policy captured per
//     amendment 5).
//   - Multi-policy: only the matched policy's per-policy counter increments.
//   - track_per_rule_stats=false → no per-policy counters allocated.
//   - Shadow + primary co-emission: shadow per-policy counter increments
//     INDEPENDENTLY of primary disposition (parallel-walk discipline).
//   - Shadow + track_per_rule_stats → per-policy shadow counters use the
//     shadow base prefix.
//   - response_code_details divergence-window: SendLocalReply path does NOT
//     thread response-code-details (envoy-go MVP no emission per amendment 11
//     + §8.12; documented in ADR-0146).
// ----------------------------------------------------------------------------

// twoPolicyLOGRBAC returns a listener-level LOG-action RBAC proto with two
// policies: `p_admin` (matches x-user=admin) + `p_guest` (matches x-user=guest).
// Used to exercise per-policy emission discrimination — only the matched
// policy's counter should increment.
func twoPolicyLOGRBAC(t *testing.T) *rbacv3.RBAC {
	t.Helper()
	mkPolicy := func(headerVal string) *rbacconfigv3.Policy {
		return &rbacconfigv3.Policy{
			Permissions: []*rbacconfigv3.Permission{{
				Rule: &rbacconfigv3.Permission_Header{
					Header: &routev3.HeaderMatcher{
						Name: "x-user",
						HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
							StringMatch: &matcherv3.StringMatcher{
								MatchPattern: &matcherv3.StringMatcher_Exact{Exact: headerVal},
							},
						},
					},
				},
			}},
			Principals: []*rbacconfigv3.Principal{
				{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
			},
		}
	}
	return &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action: rbacconfigv3.RBAC_LOG,
			Policies: map[string]*rbacconfigv3.Policy{
				"p_admin": mkPolicy("admin"),
				"p_guest": mkPolicy("guest"),
			},
		},
		RulesStatPrefix:   "myrules",
		TrackPerRuleStats: true,
	}
}

// findCounterByName scans the Registry for a counter with the given name.
// Returns nil if not present.
func findCounterByName(reg *stats.Registry, name string) *stats.Counter {
	var found *stats.Counter
	reg.Walk(func(m stats.Metric) {
		if m.Name() == name {
			if c, ok := m.(*stats.Counter); ok {
				found = c
			}
		}
	})
	return found
}

// newFilterWithCtx wires a *filter from the supplied listener-level RBAC proto
// + FactoryCtx (caller-controlled Stats Registry + StatPrefix). Used by Task 9
// tests that need to inspect specific counter names with a known HCM prefix.
func newFilterWithCtx(t *testing.T, listener *rbacv3.RBAC, ctx envoyhttp.FactoryCtx) (*filter, *rbacFakeCB) {
	t.Helper()
	factory, err := New(mustAny(t, listener), ctx)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inst := factory()
	fl, ok := inst.Decoder.(*filter)
	if !ok {
		t.Fatalf("Decoder: want *filter; got %T", inst.Decoder)
	}
	cb := &rbacFakeCB{}
	fl.SetDecoderCallbacks(cb)
	return fl, cb
}

func TestDecodeHeaders_LOGMatch_TrackPerRuleStats_AllowedPerPolicyCounterIncremented(t *testing.T) {
	// Per ADR-0146 §Decision (ii) + (iii): LOG-action with track_per_rule_stats
	// emits both the base allowed counter AND the per-policy allowed counter
	// (LOG folds into allowed per amendment 8; matched-policy captured per
	// amendment 5).
	listener := &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action: rbacconfigv3.RBAC_LOG,
			Policies: map[string]*rbacconfigv3.Policy{
				"p_log_admin": {
					Permissions: []*rbacconfigv3.Permission{{
						Rule: &rbacconfigv3.Permission_Header{
							Header: &routev3.HeaderMatcher{
								Name: "x-user",
								HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
									StringMatch: &matcherv3.StringMatcher{
										MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "admin"},
									},
								},
							},
						},
					}},
					Principals: []*rbacconfigv3.Principal{
						{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
					},
				},
			},
		},
		RulesStatPrefix:   "myrules",
		TrackPerRuleStats: true,
	}
	reg := stats.NewRegistry()
	fl, cb := newFilterWithCtx(t, listener, envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "hcm_t9"})
	beforeAllowed := fl.state.listenerRC.stats.allowed.Load()
	hdr := http.Header{"X-User": []string{"admin"}}
	status := fl.DecodeHeaders(hdr, false)
	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (LOG always-allows)", status)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on LOG path; got %+v", cb.localReply)
	}
	if got := fl.state.listenerRC.stats.allowed.Load() - beforeAllowed; got != 1 {
		t.Errorf("base allowed delta: got %d, want 1", got)
	}
	wantName := "http.hcm_t9.rbac.myrules.policy.p_log_admin.allowed"
	pp := findCounterByName(reg, wantName)
	if pp == nil {
		t.Fatalf("per-policy counter %q not registered; available names=%v", wantName, collectMetricNames(reg))
	}
	if got := pp.Load(); got != 1 {
		t.Errorf("per-policy counter %q: got %d, want 1", wantName, got)
	}
}

func TestDecodeHeaders_PerPolicyEmission_OnlyMatchedPolicyCounters_Increment(t *testing.T) {
	// Per ADR-0146 §Decision (iii): per-policy emission is keyed on the
	// MATCHED policy name. Multi-policy configs must NOT emit per-policy
	// counters for non-matched policies — only the matched policy ticks.
	listener := twoPolicyLOGRBAC(t)
	reg := stats.NewRegistry()
	fl, _ := newFilterWithCtx(t, listener, envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "hcm_multi"})
	// Send admin → matches p_admin (NOT p_guest).
	hdr := http.Header{"X-User": []string{"admin"}}
	fl.DecodeHeaders(hdr, false)
	wantAdmin := "http.hcm_multi.rbac.myrules.policy.p_admin.allowed"
	wantGuest := "http.hcm_multi.rbac.myrules.policy.p_guest.allowed"
	cAdmin := findCounterByName(reg, wantAdmin)
	if cAdmin == nil {
		t.Fatalf("matched per-policy counter %q absent; got names=%v", wantAdmin, collectMetricNames(reg))
	}
	if got := cAdmin.Load(); got != 1 {
		t.Errorf("p_admin counter: got %d, want 1", got)
	}
	// p_guest counter must NOT have been allocated (lazy + only on match).
	if cGuest := findCounterByName(reg, wantGuest); cGuest != nil {
		t.Errorf("non-matched policy counter %q must NOT be registered (lazy + matched-policy only); got Load=%d", wantGuest, cGuest.Load())
	}
}

func TestDecodeHeaders_PerPolicyEmission_TrackPerRuleStatsFalse_NoPerPolicyCounters(t *testing.T) {
	// Per ADR-0146 §Decision (iii) — gating: track_per_rule_stats=false → only
	// the 4 base counters are emitted; NO per-policy counters allocated at
	// match time. The base counter still ticks normally.
	listener := &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action: rbacconfigv3.RBAC_ALLOW,
			Policies: map[string]*rbacconfigv3.Policy{
				"p_admin": {
					Permissions: []*rbacconfigv3.Permission{{
						Rule: &rbacconfigv3.Permission_Header{
							Header: &routev3.HeaderMatcher{
								Name: "x-user",
								HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
									StringMatch: &matcherv3.StringMatcher{
										MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "admin"},
									},
								},
							},
						},
					}},
					Principals: []*rbacconfigv3.Principal{
						{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
					},
				},
			},
		},
		RulesStatPrefix: "myrules",
		// TrackPerRuleStats: false (default).
	}
	reg := stats.NewRegistry()
	fl, _ := newFilterWithCtx(t, listener, envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "hcm_off"})
	hdr := http.Header{"X-User": []string{"admin"}}
	fl.DecodeHeaders(hdr, false)
	// Base counter ticks.
	if got := fl.state.listenerRC.stats.allowed.Load(); got != 1 {
		t.Errorf("base allowed: got %d, want 1", got)
	}
	// No per-policy counter must be registered (operator-visible Registry
	// surface — the sync.Map entry-count contract moved to internal/rbac with
	// PerPolicyCounters at phase-26.3 / D-26.3-7).
	for _, n := range collectMetricNames(reg) {
		if strings.Contains(n, ".policy.") {
			t.Errorf("track_per_rule_stats=false but per-policy counter present: %q", n)
		}
	}
}

func TestDecodeHeaders_ShadowEnabled_PrimaryDispositionWins_ShadowCountersIncrement(t *testing.T) {
	// Per ADR-0146 §Decision (i): shadow walks in parallel + emits its own
	// counters but NEVER affects the primary dispatch. Configure primary=DENY
	// + match guest → primary returns Denied; shadow=ALLOW + match guest →
	// shadow returns Allowed. Verify (a) 403 fires (primary wins), (b)
	// `denied` increments, (c) `shadow_allowed` increments, (d) shadow_denied
	// does NOT.
	headerMatchGuest := func() *rbacconfigv3.Policy {
		return &rbacconfigv3.Policy{
			Permissions: []*rbacconfigv3.Permission{{
				Rule: &rbacconfigv3.Permission_Header{
					Header: &routev3.HeaderMatcher{
						Name: "x-user",
						HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
							StringMatch: &matcherv3.StringMatcher{
								MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "guest"},
							},
						},
					},
				},
			}},
			Principals: []*rbacconfigv3.Principal{
				{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
			},
		}
	}
	listener := &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action:   rbacconfigv3.RBAC_DENY,
			Policies: map[string]*rbacconfigv3.Policy{"p_deny_guest": headerMatchGuest()},
		},
		ShadowRules: &rbacconfigv3.RBAC{
			Action:   rbacconfigv3.RBAC_ALLOW,
			Policies: map[string]*rbacconfigv3.Policy{"p_allow_guest": headerMatchGuest()},
		},
		RulesStatPrefix:       "myrules",
		ShadowRulesStatPrefix: "myshadow",
	}
	reg := stats.NewRegistry()
	fl, cb := newFilterWithCtx(t, listener, envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "hcm_sp"})
	hdr := http.Header{"X-User": []string{"guest"}}
	status := fl.DecodeHeaders(hdr, false)
	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration (primary DENY + match)", status)
	}
	if cb.localReply == nil || cb.localReply.status != 403 {
		t.Fatalf("SendLocalReply: want 403; got %+v", cb.localReply)
	}
	// Primary denied increments.
	if got := fl.state.listenerRC.stats.denied.Load(); got != 1 {
		t.Errorf("primary denied: got %d, want 1", got)
	}
	if got := fl.state.listenerRC.stats.allowed.Load(); got != 0 {
		t.Errorf("primary allowed: got %d, want 0 (primary DENIED)", got)
	}
	// Shadow allowed increments (shadow=ALLOW + match → Allowed).
	if got := fl.state.listenerRC.stats.shadowAllowed.Load(); got != 1 {
		t.Errorf("shadow_allowed: got %d, want 1 (shadow ALLOW + match)", got)
	}
	if got := fl.state.listenerRC.stats.shadowDenied.Load(); got != 0 {
		t.Errorf("shadow_denied: got %d, want 0", got)
	}
}

func TestDecodeHeaders_ShadowEnabled_TrackPerRuleStats_PerPolicyShadowCountersIncrement(t *testing.T) {
	// Per ADR-0146 §Decision (i) + (iii): shadow + track_per_rule_stats →
	// per-policy shadow counter increments under the SHADOW base prefix (NOT
	// the primary prefix). Verifies the shadowBase wiring through (*rbacengine.PerPolicyCounters).Inc.
	headerMatchAdmin := func() *rbacconfigv3.Policy {
		return &rbacconfigv3.Policy{
			Permissions: []*rbacconfigv3.Permission{{
				Rule: &rbacconfigv3.Permission_Header{
					Header: &routev3.HeaderMatcher{
						Name: "x-user",
						HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
							StringMatch: &matcherv3.StringMatcher{
								MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "admin"},
							},
						},
					},
				},
			}},
			Principals: []*rbacconfigv3.Principal{
				{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
			},
		}
	}
	listener := &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action:   rbacconfigv3.RBAC_ALLOW,
			Policies: map[string]*rbacconfigv3.Policy{"p_primary": headerMatchAdmin()},
		},
		ShadowRules: &rbacconfigv3.RBAC{
			Action:   rbacconfigv3.RBAC_ALLOW,
			Policies: map[string]*rbacconfigv3.Policy{"p_shadow": headerMatchAdmin()},
		},
		RulesStatPrefix:       "myrules",
		ShadowRulesStatPrefix: "myshadow",
		TrackPerRuleStats:     true,
	}
	reg := stats.NewRegistry()
	fl, _ := newFilterWithCtx(t, listener, envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "hcm_pp"})
	hdr := http.Header{"X-User": []string{"admin"}}
	fl.DecodeHeaders(hdr, false)
	wantPrimary := "http.hcm_pp.rbac.myrules.policy.p_primary.allowed"
	wantShadow := "http.hcm_pp.rbac.myshadow.policy.p_shadow.shadow_allowed"
	if c := findCounterByName(reg, wantPrimary); c == nil {
		t.Errorf("primary per-policy counter %q absent; names=%v", wantPrimary, collectMetricNames(reg))
	} else if got := c.Load(); got != 1 {
		t.Errorf("%s: got %d, want 1", wantPrimary, got)
	}
	if c := findCounterByName(reg, wantShadow); c == nil {
		t.Errorf("shadow per-policy counter %q absent; names=%v", wantShadow, collectMetricNames(reg))
	} else if got := c.Load(); got != 1 {
		t.Errorf("%s: got %d, want 1", wantShadow, got)
	}
}

func TestDecodeHeaders_ShadowDeniedWithTrackPerRuleStats_PerPolicyShadowDeniedCounter(t *testing.T) {
	// Per ADR-0146 §Decision (i) + (iii): shadow=DENY + match + track →
	// shadow_denied base counter + per-policy `.shadow_denied` increment.
	// Primary=ALLOW + match → Allowed (request passes).
	headerMatchAdmin := func() *rbacconfigv3.Policy {
		return &rbacconfigv3.Policy{
			Permissions: []*rbacconfigv3.Permission{{
				Rule: &rbacconfigv3.Permission_Header{
					Header: &routev3.HeaderMatcher{
						Name: "x-user",
						HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
							StringMatch: &matcherv3.StringMatcher{
								MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "admin"},
							},
						},
					},
				},
			}},
			Principals: []*rbacconfigv3.Principal{
				{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
			},
		}
	}
	listener := &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action:   rbacconfigv3.RBAC_ALLOW,
			Policies: map[string]*rbacconfigv3.Policy{"p_primary": headerMatchAdmin()},
		},
		ShadowRules: &rbacconfigv3.RBAC{
			Action:   rbacconfigv3.RBAC_DENY,
			Policies: map[string]*rbacconfigv3.Policy{"p_shadow_deny": headerMatchAdmin()},
		},
		RulesStatPrefix:       "myrules",
		ShadowRulesStatPrefix: "myshadow",
		TrackPerRuleStats:     true,
	}
	reg := stats.NewRegistry()
	fl, cb := newFilterWithCtx(t, listener, envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "hcm_sd"})
	hdr := http.Header{"X-User": []string{"admin"}}
	status := fl.DecodeHeaders(hdr, false)
	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (primary ALLOW + match)", status)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on primary ALLOW + match path; got %+v", cb.localReply)
	}
	if got := fl.state.listenerRC.stats.shadowDenied.Load(); got != 1 {
		t.Errorf("shadow_denied: got %d, want 1", got)
	}
	wantShadow := "http.hcm_sd.rbac.myshadow.policy.p_shadow_deny.shadow_denied"
	if c := findCounterByName(reg, wantShadow); c == nil {
		t.Errorf("shadow per-policy counter %q absent; names=%v", wantShadow, collectMetricNames(reg))
	} else if got := c.Load(); got != 1 {
		t.Errorf("%s: got %d, want 1", wantShadow, got)
	}
}

func TestDecodeHeaders_DenyMatch_NoResponseCodeDetailsEmitted_DivergenceWindow(t *testing.T) {
	// Per ADR-0146 §Decision (iv) + SPEC §1.1 amendment 11 + §8.12: the
	// response_code_details field is NOT emitted by envoy-go MVP. The
	// SendLocalReply call exposes a 3-argument signature (status, body,
	// headers) at the DecoderFilterCallbacks interface — there is no
	// response-code-details slot. Reference Envoy v1.37.2 emits
	// `"rbac_access_denied_matched_policy[<sanitized_policy_id>]"`. This is a
	// known divergence-window documented in ADR-0146 §Consequences.
	//
	// This test pins the structural property by asserting the test-double's
	// captured args exclude any "rbac_access_denied" string in the body
	// (which is the canonical 19-byte "RBAC: access denied"; NOT the
	// response-code-details "rbac_access_denied_matched_policy[...]"). The
	// purpose is to PIN this absence: if a future framework primitive ever
	// adds response-code-details to the local-reply path, this test surfaces
	// the change immediately so the divergence-window can be closed
	// deliberately.
	fl, cb := newFilterWithRBAC(t, allowAdminRBAC())
	hdr := http.Header{"X-User": []string{"guest"}}
	fl.DecodeHeaders(hdr, false)
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want invocation on DENY; got nil")
	}
	if cb.localReply.body != denyBody {
		t.Errorf("body: got %q, want %q (deny-path body byte-exact per amendment 10)", cb.localReply.body, denyBody)
	}
	// The matched-policy-id should NOT leak into the body or headers — that
	// would be the response_code_details emission (envoy-go MVP DEFERS).
	if strings.Contains(cb.localReply.body, "rbac_access_denied_matched_policy") {
		t.Errorf("body leaks response_code_details (Envoy-only emission per amendment 11 + §8.12); got %q", cb.localReply.body)
	}
	// Headers carrier should NOT carry an x-envoy-response-code-details or
	// similar header at envoy-go MVP (no framework primitive threads it).
	for _, h := range cb.localReply.headers {
		if strings.Contains(strings.ToLower(h.Name), "response-code-details") {
			t.Errorf("response-code-details header present: %q (envoy-go MVP DEFERS per amendment 11 + §8.12)", h.Name)
		}
	}
}
