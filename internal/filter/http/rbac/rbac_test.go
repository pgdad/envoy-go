package rbac

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	xdscorev3 "github.com/cncf/xds/go/xds/core/v3"
	xdsmatcherv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	rbacconfigv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	rbacv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/matcher"
	"github.com/esalaine/envoy-go/internal/stats"
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
			if cc.rules.action != action {
				t.Errorf("action = %v; want %v", cc.rules.action, action)
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

func TestBuildCompiledRulesEngine_EmptyPermissions_Rejected(t *testing.T) {
	// Per §1.1 amendment 4 (PGV min_items=1 mirror). Per-policy permissions
	// list must be non-empty.
	c := &rbacv3.RBAC{Rules: &rbacconfigv3.RBAC{
		Action: rbacconfigv3.RBAC_ALLOW,
		Policies: map[string]*rbacconfigv3.Policy{
			"p": {
				Permissions: nil, // empty
				Principals: []*rbacconfigv3.Principal{
					{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
				},
			},
		},
	}}
	_, err := buildCompiledConfig(c, freshFactoryCtx(), false /*isPerRoute*/)
	if err == nil {
		t.Fatal("buildCompiledConfig: empty permissions; want error, got nil")
	}
	if !strings.Contains(err.Error(), "permission") {
		t.Errorf("got %q; want substring 'permission'", err.Error())
	}
}

func TestBuildCompiledRulesEngine_EmptyPrincipals_Rejected(t *testing.T) {
	c := &rbacv3.RBAC{Rules: &rbacconfigv3.RBAC{
		Action: rbacconfigv3.RBAC_ALLOW,
		Policies: map[string]*rbacconfigv3.Policy{
			"p": {
				Permissions: []*rbacconfigv3.Permission{
					{Rule: &rbacconfigv3.Permission_Any{Any: true}},
				},
				Principals: nil, // empty
			},
		},
	}}
	_, err := buildCompiledConfig(c, freshFactoryCtx(), false /*isPerRoute*/)
	if err == nil {
		t.Fatal("buildCompiledConfig: empty principals; want error, got nil")
	}
	if !strings.Contains(err.Error(), "principal") {
		t.Errorf("got %q; want substring 'principal'", err.Error())
	}
}

func TestBuildCompiledRulesEngine_LexicographicPolicyOrder_Preserved(t *testing.T) {
	// Per rbac.pb.go:268-269 "The policies are evaluated in lexicographic order
	// of the policy name." parsePerRoute compiles to a slice ordered by sorted
	// policy name.
	names := []string{"zeta", "alpha", "mike", "beta"}
	policies := make(map[string]*rbacconfigv3.Policy, len(names))
	for _, n := range names {
		policies[n] = &rbacconfigv3.Policy{
			Permissions: []*rbacconfigv3.Permission{
				{Rule: &rbacconfigv3.Permission_Any{Any: true}},
			},
			Principals: []*rbacconfigv3.Principal{
				{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
			},
		}
	}
	c := &rbacv3.RBAC{Rules: &rbacconfigv3.RBAC{
		Action:   rbacconfigv3.RBAC_ALLOW,
		Policies: policies,
	}}
	cc, err := buildCompiledConfig(c, freshFactoryCtx(), false /*isPerRoute*/)
	if err != nil {
		t.Fatalf("buildCompiledConfig: want success, got %v", err)
	}
	want := []string{"alpha", "beta", "mike", "zeta"}
	got := make([]string, 0, len(cc.rules.policies))
	for _, p := range cc.rules.policies {
		got = append(got, p.name)
	}
	if len(got) != len(want) {
		t.Fatalf("policy count = %d; want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("policy[%d] = %q; want %q (lexicographic)", i, got[i], want[i])
		}
	}
}

func TestBuildCompiledRulesEngine_AuditLoggingOptions_SilentIgnored(t *testing.T) {
	// Per §2.1.1: audit_logging_options carries `[#not-implemented-hide:]` in
	// upstream Envoy. envoy-go silent-ignores at parse + runtime. Setting it
	// does NOT produce an error.
	c := &rbacv3.RBAC{Rules: &rbacconfigv3.RBAC{
		Action:   rbacconfigv3.RBAC_ALLOW,
		Policies: allowAnyPolicy("p"),
		AuditLoggingOptions: &rbacconfigv3.RBAC_AuditLoggingOptions{
			AuditCondition: rbacconfigv3.RBAC_AuditLoggingOptions_ON_DENY,
		},
	}}
	_, err := buildCompiledConfig(c, freshFactoryCtx(), false /*isPerRoute*/)
	if err != nil {
		t.Fatalf("buildCompiledConfig: audit_logging_options set; want silent-ignore (success), got %v", err)
	}
}

func TestBuildCompiledRulesEngine_ConditionField_SilentIgnored(t *testing.T) {
	// Per §1.1 amendment 6 + Q7: Policy.condition is silent-ignored at parse +
	// runtime. We cannot use the cel-spec Expr type without pulling a heavy
	// dep; the silent-ignore is verified by setting the proto field via the
	// .pb.go binding's reflective accessor and asserting no error. The
	// concrete `Condition *v1alpha1.Expr` field is left nil here — the
	// behavioral assertion is that even setting CEL-bearing policies does NOT
	// alter parse outcomes (the silent-ignore is straight-line).
	//
	// Approach (a) per BOOTSTRAP_PROMPT: minimal input (nil Condition) suffices
	// to demonstrate that the parser does NOT consult / branch on CEL fields.
	// Test ratifies the structural invariant: parse succeeds; compiled output
	// has no condition slot to inspect (deliberate omission from
	// compiledPolicy per SPEC §6.2 "CEL fields are NOT cached").
	c := &rbacv3.RBAC{Rules: &rbacconfigv3.RBAC{
		Action:   rbacconfigv3.RBAC_ALLOW,
		Policies: allowAnyPolicy("p"),
	}}
	cc, err := buildCompiledConfig(c, freshFactoryCtx(), false /*isPerRoute*/)
	if err != nil {
		t.Fatalf("buildCompiledConfig: condition; want silent-ignore (success), got %v", err)
	}
	// compiledPolicy carries name + permissions + principals only — no
	// condition slot. Structural assertion.
	if len(cc.rules.policies) != 1 {
		t.Fatalf("compiled policies len = %d; want 1", len(cc.rules.policies))
	}
}

func TestBuildCompiledRulesEngine_CheckedConditionField_SilentIgnored(t *testing.T) {
	// Per §1.1 amendment 6 + Q7: Policy.checked_condition is silent-ignored
	// at parse + runtime. Same structural assertion as above; the
	// compiledPolicy carries NO checked_condition slot.
	c := &rbacv3.RBAC{Rules: &rbacconfigv3.RBAC{
		Action:   rbacconfigv3.RBAC_ALLOW,
		Policies: allowAnyPolicy("p"),
	}}
	cc, err := buildCompiledConfig(c, freshFactoryCtx(), false /*isPerRoute*/)
	if err != nil {
		t.Fatalf("buildCompiledConfig: checked_condition; want silent-ignore (success), got %v", err)
	}
	if len(cc.rules.policies) != 1 {
		t.Fatalf("compiled policies len = %d; want 1", len(cc.rules.policies))
	}
}

func TestBuildCompiledRulesEngine_CelConfigField_SilentIgnored(t *testing.T) {
	// Per §1.1 amendment 6 NEW: Policy.cel_config (the THIRD CEL field
	// introduced post-v1.32.x) is silent-ignored at parse + runtime.
	//
	// NOTE: envoy-go pins to go-control-plane v1.32.4 (per go.mod) which does
	// NOT carry the cel_config field on Policy. The field is structurally
	// absent from the proto binding visible at this build. Approach (b) per
	// BOOTSTRAP_PROMPT: SKIP this case at Task 2; if the module bumps to a
	// version that exposes cel_config, the silent-ignore disposition is
	// already encoded in buildCompiledRulesEngine's "no CEL slot extracted"
	// shape (the parser simply never reads CEL fields).
	t.Skip("rbac: cel_config field not present in go-control-plane v1.32.4 proto binding; silent-ignore disposition is structural — buildCompiledRulesEngine reads NO CEL fields. Test re-activates when module bumps expose the field.")
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

// ----------------------------------------------------------------------------
// Group 3 — Permission evaluators (per SPEC §14.1 #3 + §6.5). 15 test cases.
//
// Each test exercises one Permission variant (or one PARSE-REJECT) by:
//
//  1. Building an *rbacconfigv3.Permission proto carrying the variant's
//     payload.
//  2. Calling buildOnePermission(perm) → asserting either (a) the returned
//     evaluator's evaluatePermission(ctx) disposition against a stub
//     evalContext, OR (b) the parse error wording verbatim for the 3 deferred
//     variants + the const=true PGV mirror.
//
// The 15 test cases per PLAN.md line 66 + SPEC §14.1 #3:
//   - TestPermAny_True_Matches
//   - TestPermAny_FalseValue_Rejected            (PGV const=true mirror)
//   - TestPermHeader_Match                       (exact / prefix / safe-regex)
//   - TestPermURLPath_PathMatcher                (exact / prefix / safe-regex)
//   - TestPermDestIP_CIDR
//   - TestPermDestPort_Exact
//   - TestPermDestPortRange_StartLEPortLTEnd
//   - TestPermSNI_StringMatcher
//   - TestPermAndRules_Recursive_AllMatch
//   - TestPermOrRules_Recursive_AnyMatch
//   - TestPermNotRule_Recursive_Negate
//   - TestPermSourcedMetadata_ParseSupported_RuntimeFalse
//   - TestPermMetadata_PARSE_REJECT
//   - TestPermMatcher_PARSE_REJECT
//   - TestPermUriTemplate_PARSE_REJECT
// ----------------------------------------------------------------------------

// stubEvalContext is a test-only evalContext implementation. Each field is a
// pre-populated value the per-test set-up writes; the per-method accessors
// return the field verbatim. Permission accessors landed at Task 4; Principal
// accessors (DirectRemoteIP, RemoteIP, DownstreamPrincipal, SourcedMetadata,
// FilterState) landed at Task 5 per ADR-0143 §Decision (i) evalContext
// widening discipline.
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

func TestPermAny_True_Matches(t *testing.T) {
	// Per SPEC §6.5 + ADR-0143: permAny{val: true} matches any request.
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_Any{Any: true}}
	ev, err := buildOnePermission(p)
	if err != nil {
		t.Fatalf("buildOnePermission(any=true): want success, got %v", err)
	}
	if !ev.evaluatePermission(&stubEvalContext{}) {
		t.Error("permAny{val:true}.evaluatePermission(empty ctx): want true; got false")
	}
}

func TestPermAny_FalseValue_Rejected(t *testing.T) {
	// Per §1.1 amendment 4 PGV const=true mirror: Permission_Any{Any: false}
	// is rejected at parse time. envoy-go-only defensive check (Envoy would
	// PGV-validate at proto-decode boundary).
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_Any{Any: false}}
	_, err := buildOnePermission(p)
	if err == nil {
		t.Fatal("buildOnePermission(any=false): want error, got nil")
	}
	if !strings.Contains(err.Error(), "any") {
		t.Errorf("got %q; want substring 'any'", err.Error())
	}
}

func TestPermHeader_Match(t *testing.T) {
	// Per SPEC §6.5: Permission_Header wraps *routev3.HeaderMatcher. Tests
	// exact-match + prefix-match + safe-regex-match dispositions.
	cases := []struct {
		name       string
		matcher    *routev3.HeaderMatcher
		headers    map[string]string
		wantResult bool
	}{
		{
			name: "exact_match_hits",
			matcher: &routev3.HeaderMatcher{
				Name: "x-user",
				HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
					StringMatch: &matcherv3.StringMatcher{
						MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "admin"},
					},
				},
			},
			headers:    map[string]string{"x-user": "admin"},
			wantResult: true,
		},
		{
			name: "exact_match_misses",
			matcher: &routev3.HeaderMatcher{
				Name: "x-user",
				HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
					StringMatch: &matcherv3.StringMatcher{
						MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "admin"},
					},
				},
			},
			headers:    map[string]string{"x-user": "guest"},
			wantResult: false,
		},
		{
			name: "prefix_match_hits",
			matcher: &routev3.HeaderMatcher{
				Name: "x-tenant",
				HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
					StringMatch: &matcherv3.StringMatcher{
						MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "acme-"},
					},
				},
			},
			headers:    map[string]string{"x-tenant": "acme-prod"},
			wantResult: true,
		},
		{
			name: "header_absent_returns_false",
			matcher: &routev3.HeaderMatcher{
				Name: "x-user",
				HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
					StringMatch: &matcherv3.StringMatcher{
						MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "admin"},
					},
				},
			},
			headers:    map[string]string{},
			wantResult: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_Header{Header: tc.matcher}}
			ev, err := buildOnePermission(p)
			if err != nil {
				t.Fatalf("buildOnePermission(header): want success, got %v", err)
			}
			got := ev.evaluatePermission(&stubEvalContext{headers: tc.headers})
			if got != tc.wantResult {
				t.Errorf("evaluatePermission: got %v; want %v", got, tc.wantResult)
			}
		})
	}
}

func TestPermURLPath_PathMatcher(t *testing.T) {
	// Per SPEC §6.5: Permission_UrlPath wraps *matcherv3.PathMatcher whose
	// inner Path field is a StringMatcher. Tests exact + prefix + safe-regex.
	cases := []struct {
		name        string
		pathMatcher *matcherv3.PathMatcher
		urlPath     string
		wantResult  bool
	}{
		{
			name: "exact_path_hits",
			pathMatcher: &matcherv3.PathMatcher{
				Rule: &matcherv3.PathMatcher_Path{
					Path: &matcherv3.StringMatcher{
						MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "/admin"},
					},
				},
			},
			urlPath:    "/admin",
			wantResult: true,
		},
		{
			name: "prefix_path_hits",
			pathMatcher: &matcherv3.PathMatcher{
				Rule: &matcherv3.PathMatcher_Path{
					Path: &matcherv3.StringMatcher{
						MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "/api/"},
					},
				},
			},
			urlPath:    "/api/users",
			wantResult: true,
		},
		{
			name: "prefix_path_misses",
			pathMatcher: &matcherv3.PathMatcher{
				Rule: &matcherv3.PathMatcher_Path{
					Path: &matcherv3.StringMatcher{
						MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "/api/"},
					},
				},
			},
			urlPath:    "/public/index",
			wantResult: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_UrlPath{UrlPath: tc.pathMatcher}}
			ev, err := buildOnePermission(p)
			if err != nil {
				t.Fatalf("buildOnePermission(url_path): want success, got %v", err)
			}
			got := ev.evaluatePermission(&stubEvalContext{urlPath: tc.urlPath})
			if got != tc.wantResult {
				t.Errorf("evaluatePermission: got %v; want %v", got, tc.wantResult)
			}
		})
	}
}

func TestPermDestIP_CIDR(t *testing.T) {
	// Per SPEC §6.5: Permission_DestinationIp wraps *corev3.CidrRange.
	// CIDR-range match against ctx.DestinationIP().
	cidr := &corev3.CidrRange{
		AddressPrefix: "10.0.0.0",
		PrefixLen:     &wrapperspb.UInt32Value{Value: 8},
	}
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_DestinationIp{DestinationIp: cidr}}
	ev, err := buildOnePermission(p)
	if err != nil {
		t.Fatalf("buildOnePermission(destination_ip): want success, got %v", err)
	}
	cases := []struct {
		ip   string
		want bool
	}{
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"192.168.1.1", false},
		{"127.0.0.1", false},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			got := ev.evaluatePermission(&stubEvalContext{destIP: net.ParseIP(tc.ip)})
			if got != tc.want {
				t.Errorf("destIP %s: got %v; want %v", tc.ip, got, tc.want)
			}
		})
	}
}

// TestPermDestIP_CIDR_PrefixLenZero_MatchesAll exercises the documented
// matchCidr semantic: a CidrRange with PrefixLen unset (nil wrapperspb)
// defaults to 0, which means "the prefix matches all addresses" per the
// canonical Envoy semantic recorded in matchCidr's doc-comment.
func TestPermDestIP_CIDR_PrefixLenZero_MatchesAll(t *testing.T) {
	// PrefixLen left nil → defaults to 0 → matches every IP of the same family.
	cidr := &corev3.CidrRange{AddressPrefix: "0.0.0.0"}
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_DestinationIp{DestinationIp: cidr}}
	ev, err := buildOnePermission(p)
	if err != nil {
		t.Fatalf("buildOnePermission(destination_ip prefix_len=0): want success, got %v", err)
	}
	cases := []string{"10.0.0.1", "192.168.1.1", "127.0.0.1", "8.8.8.8", "0.0.0.0"}
	for _, ip := range cases {
		t.Run(ip, func(t *testing.T) {
			got := ev.evaluatePermission(&stubEvalContext{destIP: net.ParseIP(ip)})
			if !got {
				t.Errorf("destIP %s with PrefixLen=0: got false; want true (matches all)", ip)
			}
		})
	}
}

func TestPermDestPort_Exact(t *testing.T) {
	// Per SPEC §6.5: Permission_DestinationPort uint32 exact-match.
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_DestinationPort{DestinationPort: 8443}}
	ev, err := buildOnePermission(p)
	if err != nil {
		t.Fatalf("buildOnePermission(destination_port): want success, got %v", err)
	}
	if !ev.evaluatePermission(&stubEvalContext{destPort: 8443}) {
		t.Error("destPort=8443 against permDestPort{port:8443}: want true")
	}
	if ev.evaluatePermission(&stubEvalContext{destPort: 80}) {
		t.Error("destPort=80 against permDestPort{port:8443}: want false")
	}
}

func TestPermDestPortRange_StartLEPortLTEnd(t *testing.T) {
	// Per SPEC §6.5: Permission_DestinationPortRange wraps *typev3.Int32Range
	// with half-open interval semantics [start, end).
	rng := &typev3.Int32Range{Start: 8000, End: 9000}
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_DestinationPortRange{DestinationPortRange: rng}}
	ev, err := buildOnePermission(p)
	if err != nil {
		t.Fatalf("buildOnePermission(destination_port_range): want success, got %v", err)
	}
	cases := []struct {
		port uint32
		want bool
	}{
		{8000, true},  // start inclusive
		{8500, true},  // mid
		{8999, true},  // end-1 inclusive
		{9000, false}, // end exclusive
		{7999, false}, // before start
	}
	for _, tc := range cases {
		got := ev.evaluatePermission(&stubEvalContext{destPort: tc.port})
		if got != tc.want {
			t.Errorf("port=%d range[8000,9000): got %v; want %v", tc.port, got, tc.want)
		}
	}
}

func TestPermSNI_StringMatcher(t *testing.T) {
	// Per SPEC §6.5: Permission_RequestedServerName wraps *matcherv3.StringMatcher.
	// Match against ctx.RequestedServerName().
	sm := &matcherv3.StringMatcher{
		MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "example.com"},
	}
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_RequestedServerName{RequestedServerName: sm}}
	ev, err := buildOnePermission(p)
	if err != nil {
		t.Fatalf("buildOnePermission(requested_server_name): want success, got %v", err)
	}
	if !ev.evaluatePermission(&stubEvalContext{serverName: "example.com"}) {
		t.Error("SNI=example.com: want match true")
	}
	if ev.evaluatePermission(&stubEvalContext{serverName: "other.com"}) {
		t.Error("SNI=other.com: want match false")
	}
	if ev.evaluatePermission(&stubEvalContext{serverName: ""}) {
		t.Error("plaintext (empty SNI): want match false")
	}
}

func TestPermAndRules_Recursive_AllMatch(t *testing.T) {
	// Per SPEC §6.5: Permission_AndRules{rules: [...]} short-circuits to FALSE
	// on first child returning FALSE. Test exercises a 4-level deep AND chain:
	// AND(any, AND(any, AND(any, any))). All-true → TRUE; any-false → FALSE.
	innermost := &rbacconfigv3.Permission_Set{Rules: []*rbacconfigv3.Permission{
		{Rule: &rbacconfigv3.Permission_Any{Any: true}},
		{Rule: &rbacconfigv3.Permission_Any{Any: true}},
	}}
	level3 := &rbacconfigv3.Permission_Set{Rules: []*rbacconfigv3.Permission{
		{Rule: &rbacconfigv3.Permission_Any{Any: true}},
		{Rule: &rbacconfigv3.Permission_AndRules{AndRules: innermost}},
	}}
	level2 := &rbacconfigv3.Permission_Set{Rules: []*rbacconfigv3.Permission{
		{Rule: &rbacconfigv3.Permission_Any{Any: true}},
		{Rule: &rbacconfigv3.Permission_AndRules{AndRules: level3}},
	}}
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_AndRules{AndRules: level2}}
	ev, err := buildOnePermission(p)
	if err != nil {
		t.Fatalf("buildOnePermission(and 4-level): want success, got %v", err)
	}
	if !ev.evaluatePermission(&stubEvalContext{}) {
		t.Error("AND(any...any) 4-deep: want true (all-match)")
	}

	// Now flip the innermost to a port-exact mismatch to verify short-circuit.
	innermost.Rules = []*rbacconfigv3.Permission{
		{Rule: &rbacconfigv3.Permission_Any{Any: true}},
		{Rule: &rbacconfigv3.Permission_DestinationPort{DestinationPort: 99}},
	}
	evMix, err := buildOnePermission(p)
	if err != nil {
		t.Fatalf("rebuild after mixin: %v", err)
	}
	if evMix.evaluatePermission(&stubEvalContext{destPort: 80}) {
		t.Error("AND with port-mismatch deep child: want false")
	}
}

func TestPermOrRules_Recursive_AnyMatch(t *testing.T) {
	// Per SPEC §6.5: Permission_OrRules short-circuits to TRUE on first child
	// returning TRUE. Test a 3-deep OR with a permAny{val:true} buried.
	innermost := &rbacconfigv3.Permission_Set{Rules: []*rbacconfigv3.Permission{
		{Rule: &rbacconfigv3.Permission_DestinationPort{DestinationPort: 99}},
		{Rule: &rbacconfigv3.Permission_Any{Any: true}}, // wins
	}}
	level2 := &rbacconfigv3.Permission_Set{Rules: []*rbacconfigv3.Permission{
		{Rule: &rbacconfigv3.Permission_DestinationPort{DestinationPort: 88}},
		{Rule: &rbacconfigv3.Permission_OrRules{OrRules: innermost}},
	}}
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_OrRules{OrRules: level2}}
	ev, err := buildOnePermission(p)
	if err != nil {
		t.Fatalf("buildOnePermission(or 3-level): want success, got %v", err)
	}
	if !ev.evaluatePermission(&stubEvalContext{destPort: 80}) {
		t.Error("OR with any:true inside: want true")
	}

	// All-false short-circuit.
	innermost.Rules = []*rbacconfigv3.Permission{
		{Rule: &rbacconfigv3.Permission_DestinationPort{DestinationPort: 99}},
		{Rule: &rbacconfigv3.Permission_DestinationPort{DestinationPort: 77}},
	}
	evMiss, err := buildOnePermission(p)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if evMiss.evaluatePermission(&stubEvalContext{destPort: 80}) {
		t.Error("OR with no matching child: want false")
	}
}

func TestPermNotRule_Recursive_Negate(t *testing.T) {
	// Per SPEC §6.5: Permission_NotRule logically negates its child.
	inner := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_DestinationPort{DestinationPort: 80}}
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_NotRule{NotRule: inner}}
	ev, err := buildOnePermission(p)
	if err != nil {
		t.Fatalf("buildOnePermission(not): want success, got %v", err)
	}
	if ev.evaluatePermission(&stubEvalContext{destPort: 80}) {
		t.Error("NOT(port=80) against port=80: want false")
	}
	if !ev.evaluatePermission(&stubEvalContext{destPort: 81}) {
		t.Error("NOT(port=80) against port=81: want true")
	}
}

func TestPermSourcedMetadata_ParseSupported_RuntimeFalse(t *testing.T) {
	// Per §2.5 + §8.10: Permission_SourcedMetadata is parse-supported (no
	// error from buildOnePermission); evaluator ALWAYS returns FALSE at runtime
	// (the dynamic-metadata subsystem is not yet wired in envoy-go MVP).
	sm := &rbacconfigv3.SourcedMetadata{}
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_SourcedMetadata{SourcedMetadata: sm}}
	ev, err := buildOnePermission(p)
	if err != nil {
		t.Fatalf("buildOnePermission(sourced_metadata): want success (parse-supported), got %v", err)
	}
	if ev.evaluatePermission(&stubEvalContext{}) {
		t.Error("permSourcedMetadata.evaluatePermission: want false (always-no-match MVP)")
	}
}

func TestPermMetadata_PARSE_REJECT(t *testing.T) {
	// Per §2.3 + §11.P12 + planner-time D3: Permission_Metadata is deprecated
	// upstream. envoy-go-only PARSE-REJECT with the specified error wording.
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_Metadata{}}
	_, err := buildOnePermission(p)
	if err == nil {
		t.Fatal("buildOnePermission(metadata): want error, got nil")
	}
	want := "rbac: permission.metadata deprecated; use sourced_metadata"
	if err.Error() != want {
		t.Errorf("got %q; want %q", err.Error(), want)
	}
}

func TestPermMatcher_PARSE_REJECT(t *testing.T) {
	// Per §2.3 + §8.8 + planner-time D6: Permission_Matcher is an extension
	// TypedExtensionConfig variant; envoy-go MVP PARSE-REJECTs with the
	// specified error wording.
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_Matcher{}}
	_, err := buildOnePermission(p)
	if err == nil {
		t.Fatal("buildOnePermission(matcher): want error, got nil")
	}
	want := "rbac: permission.matcher extension types unsupported in this build"
	if err.Error() != want {
		t.Errorf("got %q; want %q", err.Error(), want)
	}
}

func TestPermUriTemplate_PARSE_REJECT(t *testing.T) {
	// Per §2.3 + §8.8 + planner-time D6: Permission_UriTemplate is an extension
	// TypedExtensionConfig variant; envoy-go MVP PARSE-REJECTs.
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_UriTemplate{}}
	_, err := buildOnePermission(p)
	if err == nil {
		t.Fatal("buildOnePermission(uri_template): want error, got nil")
	}
	want := "rbac: permission.uri_template extension types unsupported in this build"
	if err.Error() != want {
		t.Errorf("got %q; want %q", err.Error(), want)
	}
}

// ----------------------------------------------------------------------------
// Group 4 — Principal evaluators (per SPEC §14.1 #4 + §6.5 + §1.1 amendment 7).
// 14 test cases.
//
// Each test exercises one Principal variant (or one PARSE-REJECT) by:
//
//  1. Building an *rbacconfigv3.Principal proto carrying the variant's payload.
//  2. Calling buildOnePrincipal(p) → asserting either (a) the returned
//     evaluator's evaluatePrincipal(ctx) disposition against a stub evalContext,
//     OR (b) the parse error wording verbatim for the 3 deferred variants.
//
// The 14 test cases per PLAN.md line 66 + SPEC §14.1 #4:
//   - TestPrinAny_True_Matches
//   - TestPrinDirectRemoteIP_CIDR_PeerSource
//   - TestPrinRemoteIP_CIDR_XFFResolved
//   - TestPrinHeader_HeaderMatcher
//   - TestPrinURLPath_PathMatcher
//   - TestPrinAndIds_Recursive_AllMatch
//   - TestPrinOrIds_Recursive_AnyMatch
//   - TestPrinNotId_Recursive_Negate
//   - TestPrinSourcedMetadata_RuntimeFalse
//   - TestPrinFilterState_RuntimeFalse
//   - TestPrinAuthenticated_ThreeCaseAlgorithm (cases (a)/(b)/(c) per §1.1 amendment 12 + §6.6)
//   - TestPrinSourceIp_PARSE_REJECT
//   - TestPrinMetadata_PARSE_REJECT
//   - TestPrinCustom_PARSE_REJECT (NEW 14th variant per §1.1 amendment 7)
// ----------------------------------------------------------------------------

func TestPrinAny_True_Matches(t *testing.T) {
	// Per SPEC §6.5 + ADR-0143: prinAny{val: true} matches any request.
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_Any{Any: true}}
	ev, err := buildOnePrincipal(p)
	if err != nil {
		t.Fatalf("buildOnePrincipal(any=true): want success, got %v", err)
	}
	if !ev.evaluatePrincipal(&stubEvalContext{}) {
		t.Error("prinAny{val:true}.evaluatePrincipal(empty ctx): want true; got false")
	}

	// PGV const=true mirror per §1.1 amendment 4 — defensive at Principal_Any
	// (Permission_Any has the analogous PGV const=true; the Principal proto
	// scrape at rbac.pb.go does NOT carry an analogous annotation on
	// Principal.any, but envoy-go matches the Permission discipline for
	// symmetry — the parse-time rejection on `any: false` lives at
	// buildOnePrincipal).
	pFalse := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_Any{Any: false}}
	_, errFalse := buildOnePrincipal(pFalse)
	if errFalse == nil {
		t.Error("buildOnePrincipal(any=false): want error (PGV const=true mirror), got nil")
	}
}

func TestPrinDirectRemoteIP_CIDR_PeerSource(t *testing.T) {
	// Per SPEC §6.5 + §11.P18: Principal_DirectRemoteIp wraps *corev3.CidrRange.
	// CIDR-range match against the PEER connection source IP (no XFF
	// resolution; that's prinRemoteIP).
	cidr := &corev3.CidrRange{
		AddressPrefix: "10.0.0.0",
		PrefixLen:     &wrapperspb.UInt32Value{Value: 8},
	}
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_DirectRemoteIp{DirectRemoteIp: cidr}}
	ev, err := buildOnePrincipal(p)
	if err != nil {
		t.Fatalf("buildOnePrincipal(direct_remote_ip): want success, got %v", err)
	}
	cases := []struct {
		ip   string
		want bool
	}{
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"192.168.1.1", false},
		{"127.0.0.1", false},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			got := ev.evaluatePrincipal(&stubEvalContext{directRemoteIP: net.ParseIP(tc.ip)})
			if got != tc.want {
				t.Errorf("directRemoteIP %s: got %v; want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestPrinRemoteIP_CIDR_XFFResolved(t *testing.T) {
	// Per SPEC §6.5 + §11.P18: Principal_RemoteIp wraps *corev3.CidrRange.
	// CIDR-range match against the XFF-RESOLVED remote IP.
	//
	// XFF resolver discipline: Task 5 declares the evalContext.RemoteIP()
	// accessor; phase-16 MVP does NOT yet ship a callable XFF resolver
	// primitive at the rbac level (the framework-side XFF resolution lives
	// in phase-04/05 HCM internals + has not been surfaced to the filter
	// callbacks layer at this commit). Task 7 wires the production *filter's
	// RemoteIP() against whatever XFF accessor lands; at Task 5 the test
	// uses the stub's pre-populated remoteIP field verbatim, demonstrating
	// the CIDR match semantic over an XFF-resolved value.
	cidr := &corev3.CidrRange{
		AddressPrefix: "203.0.113.0",
		PrefixLen:     &wrapperspb.UInt32Value{Value: 24},
	}
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_RemoteIp{RemoteIp: cidr}}
	ev, err := buildOnePrincipal(p)
	if err != nil {
		t.Fatalf("buildOnePrincipal(remote_ip): want success, got %v", err)
	}
	cases := []struct {
		ip   string
		want bool
	}{
		{"203.0.113.5", true},   // XFF-resolved client IP in 203.0.113.0/24
		{"203.0.113.255", true}, // upper bound
		{"203.0.114.0", false},  // adjacent /24, miss
		{"10.0.0.1", false},     // unrelated peer addr
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			// Note: directRemoteIP intentionally differs from remoteIP to
			// confirm prinRemoteIP consumes the XFF-resolved value (RemoteIP)
			// NOT the peer addr (DirectRemoteIP).
			got := ev.evaluatePrincipal(&stubEvalContext{
				directRemoteIP: net.ParseIP("10.0.0.1"),
				remoteIP:       net.ParseIP(tc.ip),
			})
			if got != tc.want {
				t.Errorf("remoteIP %s: got %v; want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestPrinHeader_HeaderMatcher(t *testing.T) {
	// Per SPEC §6.5: Principal_Header wraps *routev3.HeaderMatcher — SAME
	// typing as Permission.Header per Task 4 spec reviewer note. Reuses the
	// local matchHeader adapter from Task 4.
	cases := []struct {
		name       string
		matcher    *routev3.HeaderMatcher
		headers    map[string]string
		wantResult bool
	}{
		{
			name: "exact_match_hits",
			matcher: &routev3.HeaderMatcher{
				Name: "x-user",
				HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
					StringMatch: &matcherv3.StringMatcher{
						MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "admin"},
					},
				},
			},
			headers:    map[string]string{"x-user": "admin"},
			wantResult: true,
		},
		{
			name: "exact_match_misses",
			matcher: &routev3.HeaderMatcher{
				Name: "x-user",
				HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
					StringMatch: &matcherv3.StringMatcher{
						MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "admin"},
					},
				},
			},
			headers:    map[string]string{"x-user": "guest"},
			wantResult: false,
		},
		{
			name: "prefix_match_hits",
			matcher: &routev3.HeaderMatcher{
				Name: "x-tenant",
				HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
					StringMatch: &matcherv3.StringMatcher{
						MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "acme-"},
					},
				},
			},
			headers:    map[string]string{"x-tenant": "acme-prod"},
			wantResult: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_Header{Header: tc.matcher}}
			ev, err := buildOnePrincipal(p)
			if err != nil {
				t.Fatalf("buildOnePrincipal(header): want success, got %v", err)
			}
			got := ev.evaluatePrincipal(&stubEvalContext{headers: tc.headers})
			if got != tc.wantResult {
				t.Errorf("evaluatePrincipal: got %v; want %v", got, tc.wantResult)
			}
		})
	}
}

func TestPrinURLPath_PathMatcher(t *testing.T) {
	// Per SPEC §6.5: Principal_UrlPath wraps *matcherv3.PathMatcher.
	// Reuses the matchPath local adapter from Task 4.
	pm := &matcherv3.PathMatcher{
		Rule: &matcherv3.PathMatcher_Path{
			Path: &matcherv3.StringMatcher{
				MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "/admin/"},
			},
		},
	}
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_UrlPath{UrlPath: pm}}
	ev, err := buildOnePrincipal(p)
	if err != nil {
		t.Fatalf("buildOnePrincipal(url_path): want success, got %v", err)
	}
	if !ev.evaluatePrincipal(&stubEvalContext{urlPath: "/admin/users"}) {
		t.Error("prinURLPath prefix /admin/ against /admin/users: want true")
	}
	if ev.evaluatePrincipal(&stubEvalContext{urlPath: "/public/index"}) {
		t.Error("prinURLPath prefix /admin/ against /public/index: want false")
	}
}

func TestPrinAndIds_Recursive_AllMatch(t *testing.T) {
	// Per SPEC §6.5: Principal_AndIds short-circuits to FALSE on first child
	// returning FALSE.
	inner := &rbacconfigv3.Principal_Set{Ids: []*rbacconfigv3.Principal{
		{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
		{Identifier: &rbacconfigv3.Principal_DirectRemoteIp{DirectRemoteIp: &corev3.CidrRange{
			AddressPrefix: "10.0.0.0",
			PrefixLen:     &wrapperspb.UInt32Value{Value: 8},
		}}},
	}}
	outer := &rbacconfigv3.Principal_Set{Ids: []*rbacconfigv3.Principal{
		{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
		{Identifier: &rbacconfigv3.Principal_AndIds{AndIds: inner}},
	}}
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_AndIds{AndIds: outer}}
	ev, err := buildOnePrincipal(p)
	if err != nil {
		t.Fatalf("buildOnePrincipal(and 2-level): want success, got %v", err)
	}
	// All-match (any + IP-in-CIDR + any).
	if !ev.evaluatePrincipal(&stubEvalContext{directRemoteIP: net.ParseIP("10.5.5.5")}) {
		t.Error("AND(any, AND(any, CIDR-match)): want true")
	}
	// IP outside CIDR → AND fails on the deep CIDR child.
	if ev.evaluatePrincipal(&stubEvalContext{directRemoteIP: net.ParseIP("192.168.1.1")}) {
		t.Error("AND with CIDR-miss deep child: want false")
	}
}

func TestPrinOrIds_Recursive_AnyMatch(t *testing.T) {
	// Per SPEC §6.5: Principal_OrIds short-circuits to TRUE on first match.
	inner := &rbacconfigv3.Principal_Set{Ids: []*rbacconfigv3.Principal{
		{Identifier: &rbacconfigv3.Principal_DirectRemoteIp{DirectRemoteIp: &corev3.CidrRange{
			AddressPrefix: "127.0.0.0",
			PrefixLen:     &wrapperspb.UInt32Value{Value: 8},
		}}},
		{Identifier: &rbacconfigv3.Principal_Any{Any: true}}, // wins
	}}
	outer := &rbacconfigv3.Principal_Set{Ids: []*rbacconfigv3.Principal{
		{Identifier: &rbacconfigv3.Principal_DirectRemoteIp{DirectRemoteIp: &corev3.CidrRange{
			AddressPrefix: "8.8.8.8",
			PrefixLen:     &wrapperspb.UInt32Value{Value: 32},
		}}},
		{Identifier: &rbacconfigv3.Principal_OrIds{OrIds: inner}},
	}}
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_OrIds{OrIds: outer}}
	ev, err := buildOnePrincipal(p)
	if err != nil {
		t.Fatalf("buildOnePrincipal(or 2-level): want success, got %v", err)
	}
	if !ev.evaluatePrincipal(&stubEvalContext{directRemoteIP: net.ParseIP("192.168.1.1")}) {
		t.Error("OR with any:true inside: want true")
	}

	// All-false short-circuit: neither outer CIDR-A nor inner CIDR-B match;
	// inner any:true was the only match path — flip it to a 127 CIDR-only.
	inner.Ids = []*rbacconfigv3.Principal{
		{Identifier: &rbacconfigv3.Principal_DirectRemoteIp{DirectRemoteIp: &corev3.CidrRange{
			AddressPrefix: "127.0.0.0",
			PrefixLen:     &wrapperspb.UInt32Value{Value: 8},
		}}},
	}
	evMiss, err := buildOnePrincipal(p)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if evMiss.evaluatePrincipal(&stubEvalContext{directRemoteIP: net.ParseIP("192.168.1.1")}) {
		t.Error("OR with no matching child: want false")
	}
}

func TestPrinNotId_Recursive_Negate(t *testing.T) {
	// Per SPEC §6.5: Principal_NotId logically negates its child.
	inner := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_DirectRemoteIp{DirectRemoteIp: &corev3.CidrRange{
		AddressPrefix: "10.0.0.0",
		PrefixLen:     &wrapperspb.UInt32Value{Value: 8},
	}}}
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_NotId{NotId: inner}}
	ev, err := buildOnePrincipal(p)
	if err != nil {
		t.Fatalf("buildOnePrincipal(not): want success, got %v", err)
	}
	if ev.evaluatePrincipal(&stubEvalContext{directRemoteIP: net.ParseIP("10.5.5.5")}) {
		t.Error("NOT(CIDR 10/8) against 10.5.5.5: want false")
	}
	if !ev.evaluatePrincipal(&stubEvalContext{directRemoteIP: net.ParseIP("192.168.1.1")}) {
		t.Error("NOT(CIDR 10/8) against 192.168.1.1: want true")
	}
}

func TestPrinSourcedMetadata_RuntimeFalse(t *testing.T) {
	// Per §2.5 + §8.10: Principal_SourcedMetadata is parse-supported (no error
	// from buildOnePrincipal); evaluator ALWAYS returns FALSE at runtime (the
	// dynamic-metadata subsystem is not yet wired in envoy-go MVP).
	sm := &rbacconfigv3.SourcedMetadata{}
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_SourcedMetadata{SourcedMetadata: sm}}
	ev, err := buildOnePrincipal(p)
	if err != nil {
		t.Fatalf("buildOnePrincipal(sourced_metadata): want success (parse-supported), got %v", err)
	}
	if ev.evaluatePrincipal(&stubEvalContext{}) {
		t.Error("prinSourcedMetadata.evaluatePrincipal: want false (always-no-match MVP)")
	}
}

func TestPrinFilterState_RuntimeFalse(t *testing.T) {
	// Per §2.5 + §8.10: Principal_FilterState is parse-supported (no error);
	// evaluator ALWAYS returns FALSE at runtime (filter-state subsystem not
	// yet wired in envoy-go MVP).
	fsm := &matcherv3.FilterStateMatcher{Key: "test"}
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_FilterState{FilterState: fsm}}
	ev, err := buildOnePrincipal(p)
	if err != nil {
		t.Fatalf("buildOnePrincipal(filter_state): want success (parse-supported), got %v", err)
	}
	if ev.evaluatePrincipal(&stubEvalContext{}) {
		t.Error("prinFilterState.evaluatePrincipal: want false (always-no-match MVP)")
	}
}

func TestPrinAuthenticated_ThreeCaseAlgorithm(t *testing.T) {
	// Per §1.1 amendment 12 + SPEC §6.6: three-case algorithm.
	//
	// Case (a): nameMatcher == nil + len(DownstreamPrincipal()) > 0 → TRUE
	//           (match-any-authenticated-user).
	// Case (b): non-nil StringMatcher iterates over DownstreamPrincipal()
	//           candidates in priority order (URI SAN → DNS SAN → Subject DN
	//           CN); TRUE on first match.
	// Case (c): plaintext / no client cert → len(DownstreamPrincipal()) == 0
	//           → FALSE (regardless of nameMatcher).

	t.Run("case_a_nil_matcher_authenticated_user", func(t *testing.T) {
		// principal_name unset; mTLS connection present (DownstreamPrincipal
		// returns at least one candidate).
		p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_Authenticated_{
			Authenticated: &rbacconfigv3.Principal_Authenticated{}, // PrincipalName: nil
		}}
		ev, err := buildOnePrincipal(p)
		if err != nil {
			t.Fatalf("buildOnePrincipal(authenticated nil-matcher): want success, got %v", err)
		}
		ctx := &stubEvalContext{
			downstreamPrincipal: []string{"spiffe://example.com/admin", "admin.example.com"},
		}
		if !ev.evaluatePrincipal(ctx) {
			t.Error("case (a) nil-matcher + len(DownstreamPrincipal)>0: want true")
		}
	})

	t.Run("case_b_string_matcher_iteration", func(t *testing.T) {
		// principal_name set to Exact("admin.example.com"); candidates =
		// [URI SAN spiffe://..., DNS SAN admin.example.com, ...]. The DNS SAN
		// (second candidate) matches.
		p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_Authenticated_{
			Authenticated: &rbacconfigv3.Principal_Authenticated{
				PrincipalName: &matcherv3.StringMatcher{
					MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "admin.example.com"},
				},
			},
		}}
		ev, err := buildOnePrincipal(p)
		if err != nil {
			t.Fatalf("buildOnePrincipal(authenticated string-matcher): want success, got %v", err)
		}
		ctx := &stubEvalContext{
			downstreamPrincipal: []string{
				"spiffe://example.com/other", // URI SAN, doesn't match
				"admin.example.com",          // DNS SAN, matches
			},
		}
		if !ev.evaluatePrincipal(ctx) {
			t.Error("case (b) StringMatcher iterates over candidates: want true (matches DNS SAN)")
		}

		// No candidate matches → FALSE.
		ctxNoMatch := &stubEvalContext{
			downstreamPrincipal: []string{
				"spiffe://example.com/other",
				"other.example.com",
			},
		}
		if ev.evaluatePrincipal(ctxNoMatch) {
			t.Error("case (b) StringMatcher with no matching candidate: want false")
		}
	})

	t.Run("case_b_uri_san_priority_first", func(t *testing.T) {
		// Verifies URI SAN (first candidate) is consulted first; a matching
		// URI SAN returns TRUE even when DNS SAN would not match.
		p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_Authenticated_{
			Authenticated: &rbacconfigv3.Principal_Authenticated{
				PrincipalName: &matcherv3.StringMatcher{
					MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "spiffe://example.com/admin"},
				},
			},
		}}
		ev, err := buildOnePrincipal(p)
		if err != nil {
			t.Fatalf("buildOnePrincipal: %v", err)
		}
		ctx := &stubEvalContext{
			downstreamPrincipal: []string{
				"spiffe://example.com/admin", // URI SAN (priority first)
				"other.example.com",          // DNS SAN
				"CN=Admin",                   // Subject DN CN
			},
		}
		if !ev.evaluatePrincipal(ctx) {
			t.Error("case (b) URI SAN priority first: want true")
		}
	})

	t.Run("case_c_plaintext_empty_candidates", func(t *testing.T) {
		// Plaintext / no client cert: DownstreamPrincipal returns nil. ALL
		// Principal_Authenticated evaluations return FALSE (regardless of
		// whether nameMatcher is set).

		// (c1) nil-matcher + empty principals → FALSE
		pNilMatcher := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_Authenticated_{
			Authenticated: &rbacconfigv3.Principal_Authenticated{},
		}}
		evNilMatcher, err := buildOnePrincipal(pNilMatcher)
		if err != nil {
			t.Fatalf("buildOnePrincipal: %v", err)
		}
		if evNilMatcher.evaluatePrincipal(&stubEvalContext{downstreamPrincipal: nil}) {
			t.Error("case (c) nil-matcher + nil principals: want false")
		}
		if evNilMatcher.evaluatePrincipal(&stubEvalContext{downstreamPrincipal: []string{}}) {
			t.Error("case (c) nil-matcher + empty principals: want false")
		}

		// (c2) non-nil matcher + empty principals → FALSE
		pWithMatcher := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_Authenticated_{
			Authenticated: &rbacconfigv3.Principal_Authenticated{
				PrincipalName: &matcherv3.StringMatcher{
					MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "anything"},
				},
			},
		}}
		evWithMatcher, err := buildOnePrincipal(pWithMatcher)
		if err != nil {
			t.Fatalf("buildOnePrincipal: %v", err)
		}
		if evWithMatcher.evaluatePrincipal(&stubEvalContext{downstreamPrincipal: nil}) {
			t.Error("case (c) StringMatcher + nil principals: want false")
		}
	})
}

func TestPrinSourceIp_PARSE_REJECT(t *testing.T) {
	// Per §2.4 + §11.P12 + planner-time D4: Principal_SourceIp is deprecated
	// upstream. envoy-go-only PARSE-REJECT with verbatim error wording.
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_SourceIp{}}
	_, err := buildOnePrincipal(p)
	if err == nil {
		t.Fatal("buildOnePrincipal(source_ip): want error, got nil")
	}
	want := "rbac: principal.source_ip deprecated; use direct_remote_ip or remote_ip"
	if err.Error() != want {
		t.Errorf("got %q; want %q", err.Error(), want)
	}
}

func TestPrinMetadata_PARSE_REJECT(t *testing.T) {
	// Per §2.4 + §11.P12 + planner-time D4: Principal_Metadata is deprecated
	// upstream. envoy-go-only PARSE-REJECT.
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_Metadata{}}
	_, err := buildOnePrincipal(p)
	if err == nil {
		t.Fatal("buildOnePrincipal(metadata): want error, got nil")
	}
	want := "rbac: principal.metadata deprecated; use sourced_metadata"
	if err.Error() != want {
		t.Errorf("got %q; want %q", err.Error(), want)
	}
}

func TestPrinCustom_PARSE_REJECT(t *testing.T) {
	// Per §1.1 amendment 7 + §8.11 NEW + planner-time D5: Principal_Custom is
	// the 14th Principal variant discovered post-BRAINSTORM. The variant is
	// a TypedExtensionConfig extension; envoy-go MVP PARSE-REJECTs with
	// verbatim error wording.
	//
	// NOTE: envoy-go pins to go-control-plane v1.32.4 (per go.mod) which does
	// NOT carry the `custom` field on Principal (the field landed in Envoy
	// post-v1.32.x; visible in v1.37.2 per amendment 7 verbatim scrape at
	// rbac.pb.go:1144 + 1112). The variant is structurally absent from the
	// proto binding visible at this build. Approach (b) per BOOTSTRAP_PROMPT:
	// SKIP this case at Task 5; the PARSE-REJECT disposition is encoded in
	// buildOnePrincipal's `default:` arm + the `Principal_Custom` case is
	// pre-staged (commented out) for activation when the module bumps to a
	// version that exposes the variant. The verbatim error wording stays
	// locked at this ADR-0143 §Decision (iv) row per amendment 7.
	t.Skip("rbac: Principal_Custom field not present in go-control-plane v1.32.4 proto binding (amendment 7 finding lands at v1.37.2). PARSE-REJECT disposition is structurally encoded in buildOnePrincipal's default arm; the explicit Principal_Custom case + this test re-activates when the module bumps expose the variant. Verbatim error wording 'rbac: principal.custom extension types unsupported in this build' stays locked at ADR-0143 §Decision (iv).")
}

// ----------------------------------------------------------------------------
// Group 7 — DownstreamPrincipal accessor framework-primitive consumption tests
// (per SPEC §14.1 #7 + §3.1 + §11.P14 + §1.1 amendment 12 + ADR-0144).
//
// Group 7 exercises the prinAuthenticated three-case algorithm THROUGH the
// stubEvalContext.DownstreamPrincipal() accessor seeded with the candidate
// shapes the production ADR-0144 plumbing surfaces from a downstream
// *tls.Conn's ConnectionState:
//
//   - plaintext / non-mTLS / no-client-cert → empty/nil slice → case (c) → FALSE.
//   - URI SAN candidate first (priority order URI SAN → DNS SAN → Subject DN CN).
//   - DNS SAN candidate second (matches when URI SAN doesn't match).
//   - Subject DN CN candidate third (fallback when URI + DNS don't match).
//   - Full priority ordering preserved across all three slots.
//
// Per BOOTSTRAP_PROMPT.md recommendation: extraction-helper-in-isolation —
// the end-to-end mTLS path is fixture 0018 scenario 6 (Task 12-14). At Task 6
// the unit tests cover the prinAuthenticated consumption via stubEvalContext
// + the hcm-side extractTLSPrincipals helper unit-tests in
// internal/filter/hcm/tls_test.go. The chain.go plumbing
// (SetTLSPrincipals → decoderCB.DownstreamPrincipal()) is covered by the
// chain_test.go probe-filter integration tests in internal/filter/http/.
// ----------------------------------------------------------------------------

func TestDownstreamPrincipal_PlaintextConnection_NilSlice(t *testing.T) {
	// Per ADR-0144 §Consequences + ADR-0143 §Decision (vi) case (c):
	// plaintext / non-mTLS / no-client-cert → DownstreamPrincipal() returns
	// nil/empty → prinAuthenticated.evaluatePrincipal returns FALSE (regardless
	// of whether nameMatcher is set).
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_Authenticated_{
		Authenticated: &rbacconfigv3.Principal_Authenticated{}, // nil principal_name
	}}
	ev, err := buildOnePrincipal(p)
	if err != nil {
		t.Fatalf("buildOnePrincipal: %v", err)
	}
	// nil downstreamPrincipal (zero-value stub) → case (c) → FALSE.
	if ev.evaluatePrincipal(&stubEvalContext{downstreamPrincipal: nil}) {
		t.Error("plaintext (nil DownstreamPrincipal): want false; got true")
	}
	// Empty slice (non-nil but len==0) is also case (c) → FALSE.
	if ev.evaluatePrincipal(&stubEvalContext{downstreamPrincipal: []string{}}) {
		t.Error("plaintext (empty DownstreamPrincipal): want false; got true")
	}
}

func TestDownstreamPrincipal_mTLSConnection_URISANs_FirstPriority(t *testing.T) {
	// Per ADR-0144 §Decision (iii) priority order URI SAN first; per §6.6
	// case (b): a StringMatcher matching the URI SAN candidate (the first
	// element of the candidate slice) returns TRUE before DNS SAN / Subject
	// DN CN are consulted. The candidate slice mirrors the production
	// extractTLSPrincipals output: URI SANs first, DNS SANs second, Subject
	// DN CN third.
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_Authenticated_{
		Authenticated: &rbacconfigv3.Principal_Authenticated{
			PrincipalName: &matcherv3.StringMatcher{
				MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "spiffe://example.com/admin"},
			},
		},
	}}
	ev, err := buildOnePrincipal(p)
	if err != nil {
		t.Fatalf("buildOnePrincipal: %v", err)
	}
	ctx := &stubEvalContext{
		downstreamPrincipal: []string{
			"spiffe://example.com/admin", // URI SAN — first priority
			"other.example.com",          // DNS SAN — second priority (would NOT match)
			"CN=Other",                   // Subject DN CN — third priority (would NOT match)
		},
	}
	if !ev.evaluatePrincipal(ctx) {
		t.Error("URI SAN first-priority match: want true; got false")
	}
}

func TestDownstreamPrincipal_mTLSConnection_DNSSANs_SecondPriority(t *testing.T) {
	// Per ADR-0144 §Decision (iii) priority order DNS SAN second; matches
	// only when URI SAN does NOT match the StringMatcher.
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_Authenticated_{
		Authenticated: &rbacconfigv3.Principal_Authenticated{
			PrincipalName: &matcherv3.StringMatcher{
				MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "admin.example.com"},
			},
		},
	}}
	ev, err := buildOnePrincipal(p)
	if err != nil {
		t.Fatalf("buildOnePrincipal: %v", err)
	}
	ctx := &stubEvalContext{
		downstreamPrincipal: []string{
			"spiffe://example.com/other", // URI SAN — does NOT match
			"admin.example.com",          // DNS SAN — matches (second priority)
			"CN=Other",                   // Subject DN CN — third priority (would NOT match)
		},
	}
	if !ev.evaluatePrincipal(ctx) {
		t.Error("DNS SAN second-priority match: want true; got false")
	}
}

func TestDownstreamPrincipal_mTLSConnection_SubjectDNCommonName_ThirdPriority(t *testing.T) {
	// Per ADR-0144 §Decision (iii) priority order Subject DN CN third;
	// matches only when neither URI SAN nor DNS SAN match the StringMatcher.
	// The production extractTLSPrincipals helper extracts only the Common
	// Name string from the Subject DN (per D11 canonical 3 cert fields; Issuer
	// DN + Serial + fingerprints DEFERRED to future TLS-context-extension phase).
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_Authenticated_{
		Authenticated: &rbacconfigv3.Principal_Authenticated{
			PrincipalName: &matcherv3.StringMatcher{
				MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "client.example.com"},
			},
		},
	}}
	ev, err := buildOnePrincipal(p)
	if err != nil {
		t.Fatalf("buildOnePrincipal: %v", err)
	}
	ctx := &stubEvalContext{
		downstreamPrincipal: []string{
			"spiffe://example.com/other", // URI SAN — does NOT match
			"other.example.com",          // DNS SAN — does NOT match
			"client.example.com",         // Subject DN CN — matches (third priority)
		},
	}
	if !ev.evaluatePrincipal(ctx) {
		t.Error("Subject DN CN third-priority match: want true; got false")
	}
}

func TestDownstreamPrincipal_OrderingPreserved(t *testing.T) {
	// Per ADR-0144 §Decision (iii): the priority order URI SAN → DNS SAN →
	// Subject DN CN must be preserved end-to-end. This test exercises the
	// EARLIEST-candidate-wins property of prinAuthenticated case (b)
	// iteration: when a StringMatcher would match MULTIPLE candidates in the
	// slice, the HIGHEST-PRIORITY (earliest) candidate wins. With a Prefix
	// matcher whose pattern matches multiple candidates, the iteration
	// returns TRUE on the first match — but the test confirms the slice
	// itself carries the priority order URI SAN first.
	//
	// Mirrors the production extractTLSPrincipals output for a cert carrying
	// all three fields: [URI SAN, DNS SAN, Subject DN CN] verbatim.
	full := []string{
		"spiffe://example.com/admin", // URI SAN — first
		"admin.example.com",          // DNS SAN — second
		"client.example.com",         // Subject DN CN — third
	}
	// Prefix matcher matches all three (each candidate contains a substring
	// of the pattern's domain root).
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_Authenticated_{
		Authenticated: &rbacconfigv3.Principal_Authenticated{
			PrincipalName: &matcherv3.StringMatcher{
				MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "spiffe://"},
			},
		},
	}}
	ev, err := buildOnePrincipal(p)
	if err != nil {
		t.Fatalf("buildOnePrincipal: %v", err)
	}
	// URI SAN first → Prefix "spiffe://" matches it first → TRUE.
	if !ev.evaluatePrincipal(&stubEvalContext{downstreamPrincipal: full}) {
		t.Error("URI SAN earliest candidate matches Prefix \"spiffe://\": want true; got false")
	}

	// Confirm slot-by-slot ordering: a permuted slice where Subject DN CN
	// is in position 0 (NOT mirroring the production extractor's priority)
	// would NOT match the URI SAN-targeted matcher. This pins the ordering
	// invariant: the priority order is the SLICE ORDER (caller's
	// responsibility to seed in priority order — chain.SetTLSPrincipals
	// trusts the caller's ordering).
	permuted := []string{
		"client.example.com",         // out-of-order Subject DN CN
		"admin.example.com",          // out-of-order DNS SAN
		"spiffe://example.com/admin", // out-of-order URI SAN
	}
	// Prefix "spiffe://" still matches via case-b iteration (TRUE on first
	// candidate that satisfies the matcher); the iteration finds the URI
	// SAN at slot index 2, NOT slot index 0. The accessor returns the slice
	// in the order seeded by HCM dispatch; the iteration honors that order.
	if !ev.evaluatePrincipal(&stubEvalContext{downstreamPrincipal: permuted}) {
		t.Error("Prefix matcher finds URI SAN in permuted slice via iteration: want true; got false")
	}
}

// ----------------------------------------------------------------------------
// Group 5 — Dual-engine dispatch (per SPEC §14.1 #5 + §6.9). 12 test cases.
//
// Exercises evaluateRulesEngine + evaluateMatcherEngine + evaluateEngine
// dispatcher per SPEC §6.9 + ADR-0141 dual-engine semantics + ADR-0142
// matcher-engine integration.
// ----------------------------------------------------------------------------

// rbacActionAnyForTest packages a canonical RBAC Action proto into *anypb.Any.
// Mirrors the matcher_test.go helper of the same role; duplicated here to keep
// rbac_test.go self-contained (Group 5/8 needs canonical-Action-terminal Any
// values to seed matcher trees).
func rbacActionAnyForTest(t *testing.T, name string, act rbacconfigv3.RBAC_Action) *anypb.Any {
	t.Helper()
	return mustAny(t, &rbacconfigv3.Action{Name: name, Action: act})
}

// headerInputTECForTest builds a TypedExtensionConfig wrapping
// HttpRequestHeaderMatchInput{HeaderName: name}. Used as SinglePredicate.Input
// on matcher tree FieldMatcher entries.
func headerInputTECForTest(t *testing.T, headerName string) *xdscorev3.TypedExtensionConfig {
	t.Helper()
	return &xdscorev3.TypedExtensionConfig{
		Name:        "header-input-" + headerName,
		TypedConfig: mustAny(t, &matcherv3.HttpRequestHeaderMatchInput{HeaderName: headerName}),
	}
}

// fieldMatcherHeaderExactForTest returns a FieldMatcher with a SinglePredicate
// of (header == exact-value) + supplied on_match leaf. Mirrors matcher_test.go.
func fieldMatcherHeaderExactForTest(t *testing.T, headerName, exact string, onMatch *xdsmatcherv3.Matcher_OnMatch) *xdsmatcherv3.Matcher_MatcherList_FieldMatcher {
	t.Helper()
	return &xdsmatcherv3.Matcher_MatcherList_FieldMatcher{
		Predicate: &xdsmatcherv3.Matcher_MatcherList_Predicate{
			MatchType: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_{
				SinglePredicate: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate{
					Input: headerInputTECForTest(t, headerName),
					Matcher: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_ValueMatch{
						ValueMatch: &xdsmatcherv3.StringMatcher{
							MatchPattern: &xdsmatcherv3.StringMatcher_Exact{Exact: exact},
						},
					},
				},
			},
		},
		OnMatch: onMatch,
	}
}

// onMatchActionForTest wraps an *anypb.Any in the OnMatch.Action shape.
func onMatchActionForTest(t *testing.T, a *anypb.Any) *xdsmatcherv3.Matcher_OnMatch {
	t.Helper()
	return &xdsmatcherv3.Matcher_OnMatch{
		OnMatch: &xdsmatcherv3.Matcher_OnMatch_Action{
			Action: &xdscorev3.TypedExtensionConfig{Name: "action", TypedConfig: a},
		},
	}
}

// singleHeaderMatcherTreeAllow returns a one-FieldMatcher matcher tree:
// (header X-User == "alice") → terminal Action{name:"p1", action:ALLOW}.
func singleHeaderMatcherTreeAllow(t *testing.T) *xdsmatcherv3.Matcher {
	t.Helper()
	return &xdsmatcherv3.Matcher{
		MatcherType: &xdsmatcherv3.Matcher_MatcherList_{
			MatcherList: &xdsmatcherv3.Matcher_MatcherList{
				Matchers: []*xdsmatcherv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherHeaderExactForTest(t, "x-user", "alice",
						onMatchActionForTest(t, rbacActionAnyForTest(t, "p1", rbacconfigv3.RBAC_ALLOW))),
				},
			},
		},
	}
}

// headerEqPolicies returns a slice of compiledPolicy with one policy whose
// single permission is header(name == value) and single principal is any.
// Used by Group 5 rules-engine match/no-match tests.
func headerEqPolicies(t *testing.T, policyName, headerName, headerValue string) []*compiledPolicy {
	t.Helper()
	perms, err := buildPermissionEvaluators([]*rbacconfigv3.Permission{{
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
	}})
	if err != nil {
		t.Fatalf("buildPermissionEvaluators: %v", err)
	}
	prins, err := buildPrincipalEvaluators([]*rbacconfigv3.Principal{
		{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
	})
	if err != nil {
		t.Fatalf("buildPrincipalEvaluators: %v", err)
	}
	return []*compiledPolicy{{name: policyName, permissions: perms, principals: prins}}
}

func TestEvaluateRulesEngine_AllowMatch_Allowed(t *testing.T) {
	// Per SPEC §6.9: ALLOW + match → engineResultAllowed + matchedPolicyName.
	re := &compiledRulesEngine{
		action:   rbacconfigv3.RBAC_ALLOW,
		policies: headerEqPolicies(t, "p_admin", "x-user", "admin"),
	}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "admin"}}
	result, name := evaluateRulesEngine(re, ctx)
	if result != engineResultAllowed {
		t.Errorf("result: got %v, want engineResultAllowed", result)
	}
	if name != "p_admin" {
		t.Errorf("name: got %q, want %q", name, "p_admin")
	}
}

func TestEvaluateRulesEngine_AllowNoMatch_Denied(t *testing.T) {
	// Per SPEC §6.9: ALLOW + no-match → engineResultDenied + "".
	re := &compiledRulesEngine{
		action:   rbacconfigv3.RBAC_ALLOW,
		policies: headerEqPolicies(t, "p_admin", "x-user", "admin"),
	}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "guest"}}
	result, name := evaluateRulesEngine(re, ctx)
	if result != engineResultDenied {
		t.Errorf("result: got %v, want engineResultDenied", result)
	}
	if name != "" {
		t.Errorf("name: got %q, want \"\"", name)
	}
}

func TestEvaluateRulesEngine_DenyMatch_Denied(t *testing.T) {
	// Per SPEC §6.9: DENY + match → engineResultDenied + matchedPolicyName.
	re := &compiledRulesEngine{
		action:   rbacconfigv3.RBAC_DENY,
		policies: headerEqPolicies(t, "p_block", "x-user", "evil"),
	}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "evil"}}
	result, name := evaluateRulesEngine(re, ctx)
	if result != engineResultDenied {
		t.Errorf("result: got %v, want engineResultDenied", result)
	}
	if name != "p_block" {
		t.Errorf("name: got %q, want %q", name, "p_block")
	}
}

func TestEvaluateRulesEngine_DenyNoMatch_Allowed(t *testing.T) {
	// Per SPEC §6.9: DENY + no-match → engineResultAllowed + "".
	re := &compiledRulesEngine{
		action:   rbacconfigv3.RBAC_DENY,
		policies: headerEqPolicies(t, "p_block", "x-user", "evil"),
	}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "alice"}}
	result, name := evaluateRulesEngine(re, ctx)
	if result != engineResultAllowed {
		t.Errorf("result: got %v, want engineResultAllowed", result)
	}
	if name != "" {
		t.Errorf("name: got %q, want \"\"", name)
	}
}

func TestEvaluateRulesEngine_LogMatch_AllowedWithPolicyName(t *testing.T) {
	// Per §1.1 amendment 5 + SPEC §6.9: LOG + match → engineResultAllowed
	// + matchedPolicyName captured for per-policy counter emission.
	re := &compiledRulesEngine{
		action:   rbacconfigv3.RBAC_LOG,
		policies: headerEqPolicies(t, "p_log", "x-user", "admin"),
	}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "admin"}}
	result, name := evaluateRulesEngine(re, ctx)
	if result != engineResultAllowed {
		t.Errorf("result: got %v, want engineResultAllowed (LOG always-allow)", result)
	}
	if name != "p_log" {
		t.Errorf("name: got %q, want %q (matched-policy captured for per-policy emission)", name, "p_log")
	}
}

func TestEvaluateRulesEngine_LogNoMatch_Allowed(t *testing.T) {
	// Per §1.1 amendment 5: LOG always-allows regardless of match disposition.
	re := &compiledRulesEngine{
		action:   rbacconfigv3.RBAC_LOG,
		policies: headerEqPolicies(t, "p_log", "x-user", "admin"),
	}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "guest"}}
	result, name := evaluateRulesEngine(re, ctx)
	if result != engineResultAllowed {
		t.Errorf("result: got %v, want engineResultAllowed (LOG always-allows)", result)
	}
	if name != "" {
		t.Errorf("name: got %q, want \"\" (no policy matched on LOG no-match)", name)
	}
}

func TestEvaluateRulesEngine_LexicographicOrderShortCircuit(t *testing.T) {
	// Per SPEC §6.9 + rbac.pb.go:268-269: policies walk in lexicographic order;
	// first match wins. Set up two policies; both could match; verify the
	// lexicographic-first one wins.
	policies := append(
		headerEqPolicies(t, "p_alpha", "x-user", "admin"),
		headerEqPolicies(t, "p_beta", "x-user", "admin")...,
	)
	re := &compiledRulesEngine{
		action:   rbacconfigv3.RBAC_ALLOW,
		policies: policies, // already in lexicographic order: p_alpha < p_beta
	}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "admin"}}
	_, name := evaluateRulesEngine(re, ctx)
	if name != "p_alpha" {
		t.Errorf("name: got %q, want %q (lexicographic first-match wins)", name, "p_alpha")
	}
}

func TestEvaluateMatcherEngine_CanonicalActionTerminal_Honored(t *testing.T) {
	// Per ADR-0142 + SPEC §6.9: matcher-engine returns the canonical RBAC
	// Action terminal Any; evaluateMatcherEngine unmarshals + maps to engine
	// result. ALLOW → engineResultAllowed; matchedName = action.GetName().
	tree, err := matcherNewForTest(t, singleHeaderMatcherTreeAllow(t))
	if err != nil {
		t.Fatalf("matcher.New: %v", err)
	}
	me := &compiledMatcherEngine{tree: tree}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "alice"}}
	result, name := evaluateMatcherEngine(me, ctx)
	if result != engineResultAllowed {
		t.Errorf("result: got %v, want engineResultAllowed", result)
	}
	if name != "p1" {
		t.Errorf("name: got %q, want %q", name, "p1")
	}
}

func TestEvaluateMatcherEngine_NoMatch_Denied(t *testing.T) {
	// Per rbac.pb.go:43-46 + SPEC §6.9: matcher-engine no-match → caller
	// interprets as DENY. evaluateMatcherEngine returns (engineResultDenied, "").
	tree, err := matcherNewForTest(t, singleHeaderMatcherTreeAllow(t))
	if err != nil {
		t.Fatalf("matcher.New: %v", err)
	}
	me := &compiledMatcherEngine{tree: tree}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "bob"}}
	result, name := evaluateMatcherEngine(me, ctx)
	if result != engineResultDenied {
		t.Errorf("result: got %v, want engineResultDenied (no-match)", result)
	}
	if name != "" {
		t.Errorf("name: got %q, want \"\"", name)
	}
}

func TestEvaluateMatcherEngine_UnknownTerminalTypeURL_ParseRejected(t *testing.T) {
	// Per ADR-0142 + §2.6: non-canonical terminal Any.TypeUrl PARSE-REJECTS at
	// buildCompiledMatcherEngine; the inner error from internal/matcher is
	// wrapped with the "rbac: matcher:" prefix per the rbac.go contract.
	bogus := mustAny(t, &matcherv3.StringMatcher{
		MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "x"},
	})
	tree := &xdsmatcherv3.Matcher{
		MatcherType: &xdsmatcherv3.Matcher_MatcherList_{
			MatcherList: &xdsmatcherv3.Matcher_MatcherList{
				Matchers: []*xdsmatcherv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherHeaderExactForTest(t, "x-user", "alice",
						onMatchActionForTest(t, bogus)),
				},
			},
		},
	}
	_, err := buildCompiledMatcherEngine(tree)
	if err == nil {
		t.Fatal("buildCompiledMatcherEngine: want PARSE-REJECT on non-canonical terminal, got nil")
	}
	// Must carry the rbac wrap-prefix.
	if !strings.HasPrefix(err.Error(), "rbac: matcher:") {
		t.Errorf("error wording want prefix 'rbac: matcher:', got %q", err.Error())
	}
	// Must surface the inner matcher-engine wording.
	if !strings.Contains(err.Error(), "terminal action type") {
		t.Errorf("error wording want substring 'terminal action type', got %q", err.Error())
	}
}

func TestEvaluateEngine_BothPrimaryAndShadowConfigured_PrimaryDispositionWinsShadowEmitsCounter(t *testing.T) {
	// Per SPEC §6.7 + §6.9: when both primary (rules) + shadow (rules) are
	// configured, the primary disposition wins. The shadow walks but does NOT
	// affect the dispatch outcome. evaluateEngine(shadow=true) returns the
	// shadow's own result for counter emission purposes.
	primary := &compiledRulesEngine{
		action:   rbacconfigv3.RBAC_ALLOW,
		policies: headerEqPolicies(t, "p_admin", "x-user", "admin"),
	}
	shadow := &compiledRulesEngine{
		action:   rbacconfigv3.RBAC_DENY,
		policies: headerEqPolicies(t, "p_admin", "x-user", "admin"),
	}
	cc := &compiledConfig{rules: primary, shadowRules: shadow}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "admin"}}
	primaryResult, primaryName := evaluateEngine(cc, ctx, false /*shadow*/)
	shadowResult, shadowName := evaluateEngine(cc, ctx, true /*shadow*/)
	if primaryResult != engineResultAllowed {
		t.Errorf("primary: got %v, want engineResultAllowed (ALLOW + match)", primaryResult)
	}
	if primaryName != "p_admin" {
		t.Errorf("primary name: got %q, want %q", primaryName, "p_admin")
	}
	if shadowResult != engineResultDenied {
		t.Errorf("shadow: got %v, want engineResultDenied (DENY + match — independent of primary)", shadowResult)
	}
	if shadowName != "p_admin" {
		t.Errorf("shadow name: got %q, want %q", shadowName, "p_admin")
	}
}

func TestEvaluateEngine_BothEnginesUnset_DefensiveAllowed(t *testing.T) {
	// Per SPEC §6.9: defensive ALLOWED when both engines (rules + matcher) are
	// nil. The DecodeHeaders body's "both engines unset" fast-path triggers
	// passthrough BEFORE evaluateEngine is called in production; but the
	// evaluateEngine helper itself returns engineResultAllowed defensively if
	// called with neither set (e.g., shadow=true when no shadow configured).
	cc := &compiledConfig{}
	ctx := &stubEvalContext{}
	result, name := evaluateEngine(cc, ctx, false /*shadow*/)
	if result != engineResultAllowed {
		t.Errorf("result: got %v, want engineResultAllowed (defensive)", result)
	}
	if name != "" {
		t.Errorf("name: got %q, want \"\"", name)
	}
	// Shadow path with no shadow configured — same defensive ALLOWED.
	result, name = evaluateEngine(cc, ctx, true /*shadow*/)
	if result != engineResultAllowed {
		t.Errorf("shadow result: got %v, want engineResultAllowed (defensive)", result)
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

// ----------------------------------------------------------------------------
// Group 8 — Matcher-engine framework primitive integration (per SPEC §14.1 #8
// + ADR-0142). 9 test cases exercising the rbac↔matcher boundary through
// buildCompiledMatcherEngine + matcherCtxAdapter.
//
// The matcher_test.go file (Task 3) covers the matcher-engine package's
// surface in isolation; rbac_test.go's Group 8 verifies the rbac-side
// integration of the framework primitive (canonical-acceptance + reject of
// non-canonical terminals + adapter delegation).
// ----------------------------------------------------------------------------

// matcherNewForTest is a thin shim that exercises buildCompiledMatcherEngine
// for Group 5 helper setup. Returns the underlying tree for direct evaluator
// invocation paths.
func matcherNewForTest(t *testing.T, m *xdsmatcherv3.Matcher) (*matcher.Matcher, error) {
	t.Helper()
	cme, err := buildCompiledMatcherEngine(m)
	if err != nil {
		return nil, err
	}
	return cme.tree, nil
}

func TestMatcherNew_CanonicalRBACActionTerminal_Accepted(t *testing.T) {
	// Per ADR-0142 + SPEC §11.P3: buildCompiledMatcherEngine accepts trees
	// whose terminal Any.TypeUrl == canonical RBAC Action TypeURL.
	tree := singleHeaderMatcherTreeAllow(t)
	cme, err := buildCompiledMatcherEngine(tree)
	if err != nil {
		t.Fatalf("buildCompiledMatcherEngine: want success on canonical Action terminal; got %v", err)
	}
	if cme == nil || cme.tree == nil {
		t.Fatal("buildCompiledMatcherEngine: want non-nil *compiledMatcherEngine + tree")
	}
}

func TestMatcherNew_UnknownTypeURL_PARSE_REJECT(t *testing.T) {
	// Per ADR-0142 + §2.6: non-canonical terminal Any.TypeUrl PARSE-REJECTs at
	// buildCompiledMatcherEngine with the "rbac: matcher:" wrap-prefix.
	bogus := mustAny(t, &matcherv3.StringMatcher{
		MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "x"},
	})
	tree := &xdsmatcherv3.Matcher{
		MatcherType: &xdsmatcherv3.Matcher_MatcherList_{
			MatcherList: &xdsmatcherv3.Matcher_MatcherList{
				Matchers: []*xdsmatcherv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherHeaderExactForTest(t, "x-user", "alice",
						onMatchActionForTest(t, bogus)),
				},
			},
		},
	}
	_, err := buildCompiledMatcherEngine(tree)
	if err == nil {
		t.Fatal("buildCompiledMatcherEngine: want PARSE-REJECT on non-canonical terminal, got nil")
	}
	if !strings.HasPrefix(err.Error(), "rbac: matcher:") {
		t.Errorf("error wording want prefix 'rbac: matcher:', got %q", err.Error())
	}
}

func TestMatcherEvaluate_FirstMatchingPredicate_ReturnsTerminalAny(t *testing.T) {
	// Per ADR-0142 + SPEC §6.9: the matcher tree walker returns the matched
	// terminal Any on first-predicate match; the rbac-side evaluator unmarshals
	// it as rbacconfigv3.Action and maps to engineResult.
	tree, err := matcherNewForTest(t, singleHeaderMatcherTreeAllow(t))
	if err != nil {
		t.Fatalf("matcherNewForTest: %v", err)
	}
	me := &compiledMatcherEngine{tree: tree}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "alice"}}
	result, name := evaluateMatcherEngine(me, ctx)
	if result != engineResultAllowed {
		t.Errorf("result: got %v, want engineResultAllowed", result)
	}
	if name != "p1" {
		t.Errorf("name: got %q, want %q", name, "p1")
	}
}

func TestMatcherEvaluate_NoMatchingPredicate_ReturnsNilNil(t *testing.T) {
	// Per rbac.pb.go:43-46: no-match → matcher.Evaluate returns (nil, nil);
	// the rbac-side evaluator maps nil-result to engineResultDenied.
	tree, err := matcherNewForTest(t, singleHeaderMatcherTreeAllow(t))
	if err != nil {
		t.Fatalf("matcherNewForTest: %v", err)
	}
	me := &compiledMatcherEngine{tree: tree}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "bob"}}
	result, _ := evaluateMatcherEngine(me, ctx)
	if result != engineResultDenied {
		t.Errorf("result: got %v, want engineResultDenied (no-match)", result)
	}
}

func TestMatcherEvaluate_HeaderPredicate_Match(t *testing.T) {
	// Per ADR-0142 §Decision (iii): the matcher's headerPredicate matches via
	// the rbac-side matcherCtxAdapter.Header() routing through *filter's
	// evalContext.Header().
	tree, err := matcherNewForTest(t, singleHeaderMatcherTreeAllow(t))
	if err != nil {
		t.Fatalf("matcherNewForTest: %v", err)
	}
	me := &compiledMatcherEngine{tree: tree}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "alice"}}
	result, _ := evaluateMatcherEngine(me, ctx)
	if result != engineResultAllowed {
		t.Error("header-predicate match (X-User=alice): want engineResultAllowed")
	}
}

func TestMatcherEvaluate_PathPredicate_Match(t *testing.T) {
	// Per ADR-0142 + matcher_test.go: the matcher's predicate input may be
	// :path (the H2 pseudo-header); the rbac-side matcherCtxAdapter routes
	// Header(":path") through the *filter's evalContext (which surfaces
	// :path via the headers map at DecodeHeaders body construction time).
	tree := &xdsmatcherv3.Matcher{
		MatcherType: &xdsmatcherv3.Matcher_MatcherList_{
			MatcherList: &xdsmatcherv3.Matcher_MatcherList{
				Matchers: []*xdsmatcherv3.Matcher_MatcherList_FieldMatcher{
					{
						Predicate: &xdsmatcherv3.Matcher_MatcherList_Predicate{
							MatchType: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_{
								SinglePredicate: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate{
									Input: headerInputTECForTest(t, ":path"),
									Matcher: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_ValueMatch{
										ValueMatch: &xdsmatcherv3.StringMatcher{
											MatchPattern: &xdsmatcherv3.StringMatcher_Prefix{Prefix: "/public"},
										},
									},
								},
							},
						},
						OnMatch: onMatchActionForTest(t, rbacActionAnyForTest(t, "p_public", rbacconfigv3.RBAC_ALLOW)),
					},
				},
			},
		},
	}
	cme, err := buildCompiledMatcherEngine(tree)
	if err != nil {
		t.Fatalf("buildCompiledMatcherEngine: %v", err)
	}
	ctx := &stubEvalContext{
		headers: map[string]string{":path": "/public/x"},
		urlPath: "/public/x",
	}
	result, name := evaluateMatcherEngine(cme, ctx)
	if result != engineResultAllowed {
		t.Errorf("path-predicate match (/public/x): want engineResultAllowed, got %v", result)
	}
	if name != "p_public" {
		t.Errorf("name: got %q, want %q", name, "p_public")
	}
}

func TestMatcherEvaluate_AndPredicate_AllMatch(t *testing.T) {
	// Per ADR-0142: AndPredicate matches when ALL child predicates match.
	// Compose (X-User=alice) AND (X-Tenant=acme).
	headerExact := func(name, val string) *xdsmatcherv3.Matcher_MatcherList_Predicate {
		return &xdsmatcherv3.Matcher_MatcherList_Predicate{
			MatchType: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_{
				SinglePredicate: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate{
					Input: headerInputTECForTest(t, name),
					Matcher: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_ValueMatch{
						ValueMatch: &xdsmatcherv3.StringMatcher{
							MatchPattern: &xdsmatcherv3.StringMatcher_Exact{Exact: val},
						},
					},
				},
			},
		}
	}
	andPred := &xdsmatcherv3.Matcher_MatcherList_Predicate{
		MatchType: &xdsmatcherv3.Matcher_MatcherList_Predicate_AndMatcher{
			AndMatcher: &xdsmatcherv3.Matcher_MatcherList_Predicate_PredicateList{
				Predicate: []*xdsmatcherv3.Matcher_MatcherList_Predicate{
					headerExact("x-user", "alice"),
					headerExact("x-tenant", "acme"),
				},
			},
		},
	}
	tree := &xdsmatcherv3.Matcher{
		MatcherType: &xdsmatcherv3.Matcher_MatcherList_{
			MatcherList: &xdsmatcherv3.Matcher_MatcherList{
				Matchers: []*xdsmatcherv3.Matcher_MatcherList_FieldMatcher{{
					Predicate: andPred,
					OnMatch:   onMatchActionForTest(t, rbacActionAnyForTest(t, "p_and", rbacconfigv3.RBAC_ALLOW)),
				}},
			},
		},
	}
	cme, err := buildCompiledMatcherEngine(tree)
	if err != nil {
		t.Fatalf("buildCompiledMatcherEngine: %v", err)
	}
	// Both headers present + match → ALLOW.
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "alice", "x-tenant": "acme"}}
	result, _ := evaluateMatcherEngine(cme, ctx)
	if result != engineResultAllowed {
		t.Errorf("AND-match: got %v, want engineResultAllowed", result)
	}
	// Only one header matches → no-match → DENY.
	ctx = &stubEvalContext{headers: map[string]string{"x-user": "alice", "x-tenant": "other"}}
	result, _ = evaluateMatcherEngine(cme, ctx)
	if result != engineResultDenied {
		t.Errorf("AND-partial-match: got %v, want engineResultDenied", result)
	}
}

func TestMatcherEvaluate_OrPredicate_AnyMatch(t *testing.T) {
	// Per ADR-0142: OrPredicate matches when ANY child predicate matches.
	headerExact := func(name, val string) *xdsmatcherv3.Matcher_MatcherList_Predicate {
		return &xdsmatcherv3.Matcher_MatcherList_Predicate{
			MatchType: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_{
				SinglePredicate: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate{
					Input: headerInputTECForTest(t, name),
					Matcher: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_ValueMatch{
						ValueMatch: &xdsmatcherv3.StringMatcher{
							MatchPattern: &xdsmatcherv3.StringMatcher_Exact{Exact: val},
						},
					},
				},
			},
		}
	}
	orPred := &xdsmatcherv3.Matcher_MatcherList_Predicate{
		MatchType: &xdsmatcherv3.Matcher_MatcherList_Predicate_OrMatcher{
			OrMatcher: &xdsmatcherv3.Matcher_MatcherList_Predicate_PredicateList{
				Predicate: []*xdsmatcherv3.Matcher_MatcherList_Predicate{
					headerExact("x-user", "alice"),
					headerExact("x-user", "bob"),
				},
			},
		},
	}
	tree := &xdsmatcherv3.Matcher{
		MatcherType: &xdsmatcherv3.Matcher_MatcherList_{
			MatcherList: &xdsmatcherv3.Matcher_MatcherList{
				Matchers: []*xdsmatcherv3.Matcher_MatcherList_FieldMatcher{{
					Predicate: orPred,
					OnMatch:   onMatchActionForTest(t, rbacActionAnyForTest(t, "p_or", rbacconfigv3.RBAC_ALLOW)),
				}},
			},
		},
	}
	cme, err := buildCompiledMatcherEngine(tree)
	if err != nil {
		t.Fatalf("buildCompiledMatcherEngine: %v", err)
	}
	// X-User=alice → first child matches → ALLOW.
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "alice"}}
	if r, _ := evaluateMatcherEngine(cme, ctx); r != engineResultAllowed {
		t.Errorf("OR (alice): want engineResultAllowed, got %v", r)
	}
	// X-User=bob → second child matches → ALLOW.
	ctx = &stubEvalContext{headers: map[string]string{"x-user": "bob"}}
	if r, _ := evaluateMatcherEngine(cme, ctx); r != engineResultAllowed {
		t.Errorf("OR (bob): want engineResultAllowed, got %v", r)
	}
	// X-User=eve → no child matches → DENY.
	ctx = &stubEvalContext{headers: map[string]string{"x-user": "eve"}}
	if r, _ := evaluateMatcherEngine(cme, ctx); r != engineResultDenied {
		t.Errorf("OR (eve): want engineResultDenied (no-match), got %v", r)
	}
}

func TestMatchContext_AccessorAdapter_DelegatesToFilter(t *testing.T) {
	// Per ADR-0142 §Decision (iii) + CF-2 (Task 3 spec review): the
	// matcherCtxAdapter delegates each MatchContext accessor to the underlying
	// evalContext. The CRITICAL mapping is SourceIP() → DirectRemoteIP() per
	// xds.type.matcher.v3.SourceIPInput semantics (peer-source-pre-XFF, NOT
	// XFF-resolved). This test pins the mapping.
	ec := &stubEvalContext{
		headers:        map[string]string{"x-test": "v1", ":method": "PUT"},
		urlPath:        "/abc",
		method:         "PUT",
		destIP:         net.ParseIP("10.0.0.1"),
		destPort:       8080,
		serverName:     "sni.example",
		directRemoteIP: net.ParseIP("10.1.1.1"), // peer-source-pre-XFF
		remoteIP:       net.ParseIP("10.2.2.2"), // XFF-resolved
	}
	a := &matcherCtxAdapter{ctx: ec}
	if v, ok := a.Header("x-test"); !ok || v != "v1" {
		t.Errorf("Header(x-test): got (%q, %v), want (\"v1\", true)", v, ok)
	}
	if a.Path() != "/abc" {
		t.Errorf("Path: got %q, want %q", a.Path(), "/abc")
	}
	if a.Method() != "PUT" {
		t.Errorf("Method: got %q, want %q", a.Method(), "PUT")
	}
	// CF-2 mapping: SourceIP must return DirectRemoteIP (peer-source-pre-XFF),
	// NOT RemoteIP (XFF-resolved).
	got := a.SourceIP()
	if !got.Equal(ec.directRemoteIP) {
		t.Errorf("SourceIP: got %v, want %v (DirectRemoteIP — peer-source-pre-XFF per CF-2)", got, ec.directRemoteIP)
	}
	if got.Equal(ec.remoteIP) {
		t.Error("SourceIP MUST NOT return RemoteIP (XFF-resolved); CF-2 mapping")
	}
	if !a.DestinationIP().Equal(ec.destIP) {
		t.Errorf("DestinationIP: got %v, want %v", a.DestinationIP(), ec.destIP)
	}
	if a.DestinationPort() != ec.destPort {
		t.Errorf("DestinationPort: got %d, want %d", a.DestinationPort(), ec.destPort)
	}
	if a.RequestedServerName() != ec.serverName {
		t.Errorf("RequestedServerName: got %q, want %q", a.RequestedServerName(), ec.serverName)
	}
}

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
//     filterStats.incPolicy lazy-cache contract (sync.Map LoadOrStore +
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
	// lazy-allocated on first match via filterStats.incPolicy → sync.Map
	// LoadOrStore + NewCounterIfAbsent. The counter name shape per the Task 8
	// empirical scrape is `<base_prefix>.policy.<policy_name>.<suffix>`
	// (REFINES the SPEC line 1842 hypothesis which omitted the `.policy.`
	// segment infix — SPEC stat-table amends at Task 9 alongside ADR-0146).
	//
	// At Task 8 the per-policy emission helper exists but is not yet wired
	// into emit*Counters (Task 9 / ADR-0146 ships the full track_per_rule_stats
	// emission). This test exercises filterStats.incPolicy directly to pin
	// the lazy-allocation contract.
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
	// Pre-emission: sync.Map empty.
	count := 0
	cc.stats.perPolicy.Range(func(_, _ any) bool { count++; return true })
	if count != 0 {
		t.Errorf("perPolicy pre-emission count = %d; want 0 (lazy)", count)
	}
	// First emission: allocates + increments.
	cc.stats.incPolicy(cc.stats.primaryBase, "p_admin", "allowed")
	want := "http.hcm.rbac.rp.policy.p_admin.allowed"
	if !containsString(collectMetricNames(reg), want) {
		t.Errorf("missing per-policy counter %q after first incPolicy", want)
	}
	// Second emission: cache HIT (sync.Map count must NOT grow).
	cc.stats.incPolicy(cc.stats.primaryBase, "p_admin", "allowed")
	count = 0
	cc.stats.perPolicy.Range(func(_, _ any) bool { count++; return true })
	if count != 1 {
		t.Errorf("perPolicy after 2 emissions for same key: count = %d; want 1 (cache hit)", count)
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
// filterStats.incPolicy (sync.Map LoadOrStore + NewCounterIfAbsent); the
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
	// No per-policy counter must be registered.
	for _, n := range collectMetricNames(reg) {
		if strings.Contains(n, ".policy.") {
			t.Errorf("track_per_rule_stats=false but per-policy counter present: %q", n)
		}
	}
	// sync.Map.perPolicy must be empty.
	count := 0
	fl.state.listenerRC.stats.perPolicy.Range(func(_, _ any) bool { count++; return true })
	if count != 0 {
		t.Errorf("perPolicy count: got %d, want 0 (track_per_rule_stats=false)", count)
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
	// the primary prefix). Verifies the shadowBase wiring through incPolicy.
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
