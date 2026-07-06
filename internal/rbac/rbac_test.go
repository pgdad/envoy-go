package rbac

import (
	"net"
	"strings"
	"testing"

	xdscorev3 "github.com/cncf/xds/go/xds/core/v3"
	xdsmatcherv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	rbacconfigv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/matcher"
)

// mustAnyEngine packages a proto into an *anypb.Any. Local to this file;
// mirrors the phase-13/14/15 test helper precedent.
func mustAnyEngine(t *testing.T, msg proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

// rbacActionAnyForTest packages a canonical RBAC Action proto into *anypb.Any.
func rbacActionAnyForTest(t *testing.T, name string, act rbacconfigv3.RBAC_Action) *anypb.Any {
	t.Helper()
	return mustAnyEngine(t, &rbacconfigv3.Action{Name: name, Action: act})
}

// headerInputTECForTest builds a TypedExtensionConfig wrapping
// HttpRequestHeaderMatchInput{HeaderName: name}.
func headerInputTECForTest(t *testing.T, headerName string) *xdscorev3.TypedExtensionConfig {
	t.Helper()
	return &xdscorev3.TypedExtensionConfig{
		Name:        "header-input-" + headerName,
		TypedConfig: mustAnyEngine(t, &matcherv3.HttpRequestHeaderMatchInput{HeaderName: headerName}),
	}
}

// fieldMatcherHeaderExactForTest returns a FieldMatcher with a SinglePredicate
// of (header == exact-value) + supplied on_match leaf.
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

// matcherNewForTest builds a *matcher.Matcher via BuildMatcherEngine.
func matcherNewForTest(t *testing.T, m *xdsmatcherv3.Matcher) (*matcher.Matcher, error) {
	t.Helper()
	cme, err := BuildMatcherEngine(m, ProfileHTTP)
	if err != nil {
		return nil, err
	}
	return cme.tree, nil
}

// headerEqPolicies returns a slice of compiledPolicy with one policy whose
// single permission is header(name == value) and single principal is any.
// Used by rules-engine match/no-match tests.
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
	}}, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildPermissionEvaluators: %v", err)
	}
	prins, err := buildPrincipalEvaluators([]*rbacconfigv3.Principal{
		{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
	}, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildPrincipalEvaluators: %v", err)
	}
	return []*compiledPolicy{{name: policyName, permissions: perms, principals: prins}}
}

// ----------------------------------------------------------------------------
// BuildRulesEngine tests (per SPEC §14.1 #1 engine subset + §6.5 + §1.1
// amendments 1-6). Mirror of HTTP rbac TestBuildCompiledRulesEngine_* tests
// adapted to call BuildRulesEngine directly.
// ----------------------------------------------------------------------------

func TestBuildRulesEngine_EmptyPermissions_Rejected(t *testing.T) {
	// Per §1.1 amendment 4 (PGV min_items=1 mirror). Per-policy permissions
	// list must be non-empty.
	r := &rbacconfigv3.RBAC{
		Action: rbacconfigv3.RBAC_ALLOW,
		Policies: map[string]*rbacconfigv3.Policy{
			"p": {
				Permissions: nil, // empty
				Principals: []*rbacconfigv3.Principal{
					{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
				},
			},
		},
	}
	_, err := BuildRulesEngine(r, ProfileHTTP)
	if err == nil {
		t.Fatal("BuildRulesEngine: empty permissions; want error, got nil")
	}
	if !strings.Contains(err.Error(), "permission") {
		t.Errorf("got %q; want substring 'permission'", err.Error())
	}
}

func TestBuildRulesEngine_EmptyPrincipals_Rejected(t *testing.T) {
	r := &rbacconfigv3.RBAC{
		Action: rbacconfigv3.RBAC_ALLOW,
		Policies: map[string]*rbacconfigv3.Policy{
			"p": {
				Permissions: []*rbacconfigv3.Permission{
					{Rule: &rbacconfigv3.Permission_Any{Any: true}},
				},
				Principals: nil, // empty
			},
		},
	}
	_, err := BuildRulesEngine(r, ProfileHTTP)
	if err == nil {
		t.Fatal("BuildRulesEngine: empty principals; want error, got nil")
	}
	if !strings.Contains(err.Error(), "principal") {
		t.Errorf("got %q; want substring 'principal'", err.Error())
	}
}

func TestBuildRulesEngine_LexicographicPolicyOrder_Preserved(t *testing.T) {
	// Per rbac.pb.go:268-269 "The policies are evaluated in lexicographic order
	// of the policy name."
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
	r := &rbacconfigv3.RBAC{
		Action:   rbacconfigv3.RBAC_ALLOW,
		Policies: policies,
	}
	re, err := BuildRulesEngine(r, ProfileHTTP)
	if err != nil {
		t.Fatalf("BuildRulesEngine: want success, got %v", err)
	}
	want := []string{"alpha", "beta", "mike", "zeta"}
	got := make([]string, 0, len(re.policies))
	for _, p := range re.policies {
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

func TestBuildRulesEngine_AuditLoggingOptions_SilentIgnored(t *testing.T) {
	// Per §2.1.1: audit_logging_options silent-ignored at parse.
	r := &rbacconfigv3.RBAC{
		Action: rbacconfigv3.RBAC_ALLOW,
		Policies: map[string]*rbacconfigv3.Policy{
			"p": {
				Permissions: []*rbacconfigv3.Permission{{Rule: &rbacconfigv3.Permission_Any{Any: true}}},
				Principals:  []*rbacconfigv3.Principal{{Identifier: &rbacconfigv3.Principal_Any{Any: true}}},
			},
		},
		AuditLoggingOptions: &rbacconfigv3.RBAC_AuditLoggingOptions{
			AuditCondition: rbacconfigv3.RBAC_AuditLoggingOptions_ON_DENY,
		},
	}
	_, err := BuildRulesEngine(r, ProfileHTTP)
	if err != nil {
		t.Fatalf("BuildRulesEngine: audit_logging_options set; want silent-ignore (success), got %v", err)
	}
}

func TestBuildRulesEngine_ConditionField_SilentIgnored(t *testing.T) {
	// Per §1.1 amendment 6 + Q7: Policy.condition is silent-ignored. The
	// structural invariant: compiled output has NO condition slot in compiledPolicy.
	r := &rbacconfigv3.RBAC{
		Action: rbacconfigv3.RBAC_ALLOW,
		Policies: map[string]*rbacconfigv3.Policy{
			"p": {
				Permissions: []*rbacconfigv3.Permission{{Rule: &rbacconfigv3.Permission_Any{Any: true}}},
				Principals:  []*rbacconfigv3.Principal{{Identifier: &rbacconfigv3.Principal_Any{Any: true}}},
			},
		},
	}
	re, err := BuildRulesEngine(r, ProfileHTTP)
	if err != nil {
		t.Fatalf("BuildRulesEngine: condition; want silent-ignore (success), got %v", err)
	}
	// compiledPolicy carries name + permissions + principals only — no
	// condition slot. Structural assertion.
	if len(re.policies) != 1 {
		t.Fatalf("compiled policies len = %d; want 1", len(re.policies))
	}
}

func TestBuildRulesEngine_CheckedConditionField_SilentIgnored(t *testing.T) {
	// Per §1.1 amendment 6 + Q7: Policy.checked_condition is silent-ignored.
	r := &rbacconfigv3.RBAC{
		Action: rbacconfigv3.RBAC_ALLOW,
		Policies: map[string]*rbacconfigv3.Policy{
			"p": {
				Permissions: []*rbacconfigv3.Permission{{Rule: &rbacconfigv3.Permission_Any{Any: true}}},
				Principals:  []*rbacconfigv3.Principal{{Identifier: &rbacconfigv3.Principal_Any{Any: true}}},
			},
		},
	}
	re, err := BuildRulesEngine(r, ProfileHTTP)
	if err != nil {
		t.Fatalf("BuildRulesEngine: checked_condition; want silent-ignore (success), got %v", err)
	}
	if len(re.policies) != 1 {
		t.Fatalf("compiled policies len = %d; want 1", len(re.policies))
	}
}

func TestBuildRulesEngine_CelConfigField_SilentIgnored(t *testing.T) {
	// Per §1.1 amendment 6 NEW: Policy.cel_config silent-ignored. The field is
	// structurally absent from go-control-plane v1.32.4; skip per amendment 7 approach (b).
	t.Skip("rbac: cel_config field not present in go-control-plane v1.32.4 proto binding; silent-ignore disposition is structural — BuildRulesEngine reads NO CEL fields. Test re-activates when module bumps expose the field.")
}

func TestBuildRulesEngine_AllThreeActionEnumValues_Accepted(t *testing.T) {
	// Per §1.1 amendment 4: all three action enum values (ALLOW/DENY/LOG) pass
	// the PGV-mirror check; invalid values are rejected.
	for _, action := range []rbacconfigv3.RBAC_Action{
		rbacconfigv3.RBAC_ALLOW,
		rbacconfigv3.RBAC_DENY,
		rbacconfigv3.RBAC_LOG,
	} {
		r := &rbacconfigv3.RBAC{
			Action: action,
			Policies: map[string]*rbacconfigv3.Policy{
				"p": {
					Permissions: []*rbacconfigv3.Permission{{Rule: &rbacconfigv3.Permission_Any{Any: true}}},
					Principals:  []*rbacconfigv3.Principal{{Identifier: &rbacconfigv3.Principal_Any{Any: true}}},
				},
			},
		}
		_, err := BuildRulesEngine(r, ProfileHTTP)
		if err != nil {
			t.Errorf("BuildRulesEngine(action=%v): want success, got %v", action, err)
		}
	}
}

func TestBuildRulesEngine_InvalidActionEnum_Rejected(t *testing.T) {
	// Per §1.1 amendment 4: invalid action enum rejected at parse time with
	// byte-stable "rbac: invalid action" error.
	r := &rbacconfigv3.RBAC{
		Action: rbacconfigv3.RBAC_Action(99), // out-of-range
		Policies: map[string]*rbacconfigv3.Policy{
			"p": {
				Permissions: []*rbacconfigv3.Permission{{Rule: &rbacconfigv3.Permission_Any{Any: true}}},
				Principals:  []*rbacconfigv3.Principal{{Identifier: &rbacconfigv3.Principal_Any{Any: true}}},
			},
		},
	}
	_, err := BuildRulesEngine(r, ProfileHTTP)
	if err == nil {
		t.Fatal("BuildRulesEngine: invalid action; want error, got nil")
	}
	if !strings.Contains(err.Error(), "rbac: invalid action") {
		t.Errorf("got %q; want substring 'rbac: invalid action'", err.Error())
	}
}

// ----------------------------------------------------------------------------
// CompiledRulesEngine.Evaluate tests (per SPEC §14.1 #5 + §6.9). Mirrors
// the HTTP TestEvaluateRulesEngine_* tests adapted to exported names.
// ----------------------------------------------------------------------------

func TestEvaluateRulesEngine_AllowMatch_Allowed(t *testing.T) {
	// Per SPEC §6.9: ALLOW + match → Allowed + matchedPolicyName.
	re := &CompiledRulesEngine{
		action:   rbacconfigv3.RBAC_ALLOW,
		policies: headerEqPolicies(t, "p_admin", "x-user", "admin"),
	}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "admin"}}
	result, name := re.Evaluate(ctx)
	if result != Allowed {
		t.Errorf("result: got %v, want Allowed", result)
	}
	if name != "p_admin" {
		t.Errorf("name: got %q, want %q", name, "p_admin")
	}
}

func TestEvaluateRulesEngine_AllowNoMatch_Denied(t *testing.T) {
	// Per SPEC §6.9: ALLOW + no-match → Denied + "".
	re := &CompiledRulesEngine{
		action:   rbacconfigv3.RBAC_ALLOW,
		policies: headerEqPolicies(t, "p_admin", "x-user", "admin"),
	}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "guest"}}
	result, name := re.Evaluate(ctx)
	if result != Denied {
		t.Errorf("result: got %v, want Denied", result)
	}
	if name != "" {
		t.Errorf("name: got %q, want \"\"", name)
	}
}

func TestEvaluateRulesEngine_DenyMatch_Denied(t *testing.T) {
	// Per SPEC §6.9: DENY + match → Denied + matchedPolicyName.
	re := &CompiledRulesEngine{
		action:   rbacconfigv3.RBAC_DENY,
		policies: headerEqPolicies(t, "p_block", "x-user", "evil"),
	}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "evil"}}
	result, name := re.Evaluate(ctx)
	if result != Denied {
		t.Errorf("result: got %v, want Denied", result)
	}
	if name != "p_block" {
		t.Errorf("name: got %q, want %q", name, "p_block")
	}
}

func TestEvaluateRulesEngine_DenyNoMatch_Allowed(t *testing.T) {
	// Per SPEC §6.9: DENY + no-match → Allowed + "".
	re := &CompiledRulesEngine{
		action:   rbacconfigv3.RBAC_DENY,
		policies: headerEqPolicies(t, "p_block", "x-user", "evil"),
	}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "alice"}}
	result, name := re.Evaluate(ctx)
	if result != Allowed {
		t.Errorf("result: got %v, want Allowed", result)
	}
	if name != "" {
		t.Errorf("name: got %q, want \"\"", name)
	}
}

func TestEvaluateRulesEngine_LogMatch_AllowedWithPolicyName(t *testing.T) {
	// Per §1.1 amendment 5 + SPEC §6.9: LOG + match → Allowed + matchedPolicyName
	// captured for per-policy counter emission.
	re := &CompiledRulesEngine{
		action:   rbacconfigv3.RBAC_LOG,
		policies: headerEqPolicies(t, "p_log", "x-user", "admin"),
	}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "admin"}}
	result, name := re.Evaluate(ctx)
	if result != Allowed {
		t.Errorf("result: got %v, want Allowed (LOG always-allow)", result)
	}
	if name != "p_log" {
		t.Errorf("name: got %q, want %q (matched-policy captured for per-policy emission)", name, "p_log")
	}
}

func TestEvaluateRulesEngine_LogNoMatch_Allowed(t *testing.T) {
	// Per §1.1 amendment 5: LOG always-allows regardless of match disposition.
	re := &CompiledRulesEngine{
		action:   rbacconfigv3.RBAC_LOG,
		policies: headerEqPolicies(t, "p_log", "x-user", "admin"),
	}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "guest"}}
	result, name := re.Evaluate(ctx)
	if result != Allowed {
		t.Errorf("result: got %v, want Allowed (LOG always-allows)", result)
	}
	if name != "" {
		t.Errorf("name: got %q, want \"\" (no policy matched on LOG no-match)", name)
	}
}

func TestEvaluateRulesEngine_LexicographicOrderShortCircuit(t *testing.T) {
	// Per SPEC §6.9 + rbac.pb.go:268-269: policies walk in lexicographic order;
	// first match wins.
	policies := append(
		headerEqPolicies(t, "p_alpha", "x-user", "admin"),
		headerEqPolicies(t, "p_beta", "x-user", "admin")...,
	)
	re := &CompiledRulesEngine{
		action:   rbacconfigv3.RBAC_ALLOW,
		policies: policies, // already in lexicographic order: p_alpha < p_beta
	}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "admin"}}
	_, name := re.Evaluate(ctx)
	if name != "p_alpha" {
		t.Errorf("name: got %q, want %q (lexicographic first-match wins)", name, "p_alpha")
	}
}

// ----------------------------------------------------------------------------
// BuildMatcherEngine + CompiledMatcherEngine.Evaluate tests (per SPEC §14.1 #5
// + ADR-0142). Mirrors HTTP TestMatcherNew_* + TestEvaluateMatcherEngine_* +
// TestMatcherEvaluate_* adapted to exported names.
// ----------------------------------------------------------------------------

func TestBuildMatcherEngine_CanonicalRBACActionTerminal_Accepted(t *testing.T) {
	// Per ADR-0142 + SPEC §11.P3: BuildMatcherEngine accepts trees whose
	// terminal Any.TypeUrl == canonical RBAC Action TypeURL.
	tree := singleHeaderMatcherTreeAllow(t)
	cme, err := BuildMatcherEngine(tree, ProfileHTTP)
	if err != nil {
		t.Fatalf("BuildMatcherEngine: want success on canonical Action terminal; got %v", err)
	}
	if cme == nil || cme.tree == nil {
		t.Fatal("BuildMatcherEngine: want non-nil *CompiledMatcherEngine + tree")
	}
}

func TestBuildMatcherEngine_UnknownTypeURL_PARSE_REJECT(t *testing.T) {
	// Per ADR-0142 + §2.6: non-canonical terminal Any.TypeUrl PARSE-REJECTs
	// with the "rbac: matcher:" wrap-prefix.
	bogus := mustAnyEngine(t, &matcherv3.StringMatcher{
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
	_, err := BuildMatcherEngine(tree, ProfileHTTP)
	if err == nil {
		t.Fatal("BuildMatcherEngine: want PARSE-REJECT on non-canonical terminal, got nil")
	}
	if !strings.HasPrefix(err.Error(), "rbac: matcher:") {
		t.Errorf("error wording want prefix 'rbac: matcher:', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "terminal action type") {
		t.Errorf("error wording want substring 'terminal action type', got %q", err.Error())
	}
}

func TestEvaluateMatcherEngine_CanonicalActionTerminal_Honored(t *testing.T) {
	// Per ADR-0142 + SPEC §6.9: matcher-engine returns the canonical RBAC
	// Action terminal Any; Evaluate unmarshals + maps to EngineResult.
	// ALLOW → Allowed; matchedName = action.GetName().
	tree, err := matcherNewForTest(t, singleHeaderMatcherTreeAllow(t))
	if err != nil {
		t.Fatalf("matcher.New: %v", err)
	}
	me := &CompiledMatcherEngine{tree: tree}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "alice"}}
	result, name := me.Evaluate(ctx)
	if result != Allowed {
		t.Errorf("result: got %v, want Allowed", result)
	}
	if name != "p1" {
		t.Errorf("name: got %q, want %q", name, "p1")
	}
}

func TestEvaluateMatcherEngine_NoMatch_Denied(t *testing.T) {
	// Per rbac.pb.go:43-46 + SPEC §6.9: matcher-engine no-match → Denied.
	tree, err := matcherNewForTest(t, singleHeaderMatcherTreeAllow(t))
	if err != nil {
		t.Fatalf("matcher.New: %v", err)
	}
	me := &CompiledMatcherEngine{tree: tree}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "bob"}}
	result, name := me.Evaluate(ctx)
	if result != Denied {
		t.Errorf("result: got %v, want Denied (no-match)", result)
	}
	if name != "" {
		t.Errorf("name: got %q, want \"\"", name)
	}
}

func TestEvaluateMatcherEngine_MalformedTerminalBytes_Denied(t *testing.T) {
	// Per rbac.go Evaluate comment: if the terminal Any has the canonical TypeURL
	// (passes BuildMatcherEngine's PARSE-REJECT guard) but its Value bytes are
	// malformed protobuf, actionAny.UnmarshalTo returns an error → Evaluate falls
	// through to the defensive Denied branch (lines 236-241 in rbac.go).
	//
	// Construct the tree with a canonical TypeURL but deliberately corrupted Value
	// bytes. matcher.New accepts it (TypeURL is in the allow-list); UnmarshalTo
	// fails at evaluation time because the bytes are not valid protobuf for
	// rbacconfigv3.Action.
	malformed := &anypb.Any{
		TypeUrl: actionTypeURL,
		Value:   []byte{0xFF, 0xFE}, // not valid protobuf wire format for Action
	}
	tree := &xdsmatcherv3.Matcher{
		MatcherType: &xdsmatcherv3.Matcher_MatcherList_{
			MatcherList: &xdsmatcherv3.Matcher_MatcherList{
				Matchers: []*xdsmatcherv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherHeaderExactForTest(t, "x-user", "alice",
						onMatchActionForTest(t, malformed)),
				},
			},
		},
	}
	cme, err := BuildMatcherEngine(tree, ProfileHTTP)
	if err != nil {
		t.Fatalf("BuildMatcherEngine: want success (canonical TypeURL accepted), got %v", err)
	}
	// Evaluate with a matching context — the tree matches, returns the Any, then
	// UnmarshalTo fails on the malformed bytes → defensive Denied with empty name.
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "alice"}}
	result, name := cme.Evaluate(ctx)
	if result != Denied {
		t.Errorf("result: got %v, want Denied (defensive UnmarshalTo failure)", result)
	}
	if name != "" {
		t.Errorf("name: got %q, want \"\" (defensive branch returns empty name)", name)
	}
}

func TestMatcherEvaluate_FirstMatchingPredicate_ReturnsTerminalAny(t *testing.T) {
	// Per ADR-0142 + SPEC §6.9: the matcher tree walker returns the matched
	// terminal Any on first-predicate match.
	tree, err := matcherNewForTest(t, singleHeaderMatcherTreeAllow(t))
	if err != nil {
		t.Fatalf("matcherNewForTest: %v", err)
	}
	me := &CompiledMatcherEngine{tree: tree}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "alice"}}
	result, name := me.Evaluate(ctx)
	if result != Allowed {
		t.Errorf("result: got %v, want Allowed", result)
	}
	if name != "p1" {
		t.Errorf("name: got %q, want %q", name, "p1")
	}
}

func TestMatcherEvaluate_NoMatchingPredicate_ReturnsNilNil(t *testing.T) {
	// Per rbac.pb.go:43-46: no-match → matcher.Evaluate returns (nil, nil);
	// the rbac-side evaluator maps nil-result to Denied.
	tree, err := matcherNewForTest(t, singleHeaderMatcherTreeAllow(t))
	if err != nil {
		t.Fatalf("matcherNewForTest: %v", err)
	}
	me := &CompiledMatcherEngine{tree: tree}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "bob"}}
	result, _ := me.Evaluate(ctx)
	if result != Denied {
		t.Errorf("result: got %v, want Denied (no-match)", result)
	}
}

func TestMatcherEvaluate_HeaderPredicate_Match(t *testing.T) {
	// Per ADR-0142 §Decision (iii): the matcher's headerPredicate matches via
	// the matcherCtxAdapter.Header() routing through EvalContext.Header().
	tree, err := matcherNewForTest(t, singleHeaderMatcherTreeAllow(t))
	if err != nil {
		t.Fatalf("matcherNewForTest: %v", err)
	}
	me := &CompiledMatcherEngine{tree: tree}
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "alice"}}
	result, _ := me.Evaluate(ctx)
	if result != Allowed {
		t.Error("header-predicate match (X-User=alice): want Allowed")
	}
}

func TestMatcherEvaluate_PathPredicate_Match(t *testing.T) {
	// Per ADR-0142: the matcher's predicate input may be :path; the rbac-side
	// matcherCtxAdapter routes Header(":path") through EvalContext.
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
	cme, err := BuildMatcherEngine(tree, ProfileHTTP)
	if err != nil {
		t.Fatalf("BuildMatcherEngine: %v", err)
	}
	ctx := &stubEvalContext{
		headers: map[string]string{":path": "/public/x"},
		urlPath: "/public/x",
	}
	result, name := cme.Evaluate(ctx)
	if result != Allowed {
		t.Errorf("path-predicate match (/public/x): want Allowed, got %v", result)
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
	cme, err := BuildMatcherEngine(tree, ProfileHTTP)
	if err != nil {
		t.Fatalf("BuildMatcherEngine: %v", err)
	}
	// Both headers present + match → ALLOW.
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "alice", "x-tenant": "acme"}}
	result, _ := cme.Evaluate(ctx)
	if result != Allowed {
		t.Errorf("AND-match: got %v, want Allowed", result)
	}
	// Only one header matches → no-match → DENY.
	ctx = &stubEvalContext{headers: map[string]string{"x-user": "alice", "x-tenant": "other"}}
	result, _ = cme.Evaluate(ctx)
	if result != Denied {
		t.Errorf("AND-partial-match: got %v, want Denied", result)
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
	cme, err := BuildMatcherEngine(tree, ProfileHTTP)
	if err != nil {
		t.Fatalf("BuildMatcherEngine: %v", err)
	}
	// X-User=alice → first child matches → ALLOW.
	ctx := &stubEvalContext{headers: map[string]string{"x-user": "alice"}}
	if r, _ := cme.Evaluate(ctx); r != Allowed {
		t.Errorf("OR (alice): want Allowed, got %v", r)
	}
	// X-User=bob → second child matches → ALLOW.
	ctx = &stubEvalContext{headers: map[string]string{"x-user": "bob"}}
	if r, _ := cme.Evaluate(ctx); r != Allowed {
		t.Errorf("OR (bob): want Allowed, got %v", r)
	}
	// X-User=eve → no child matches → DENY.
	ctx = &stubEvalContext{headers: map[string]string{"x-user": "eve"}}
	if r, _ := cme.Evaluate(ctx); r != Denied {
		t.Errorf("OR (eve): want Denied (no-match), got %v", r)
	}
}

func TestMatchContext_AccessorAdapter_DelegatesToEvalContext(t *testing.T) {
	// Per ADR-0142 §Decision (iii) + CF-2 (Task 3 spec review): the
	// matcherCtxAdapter delegates each MatchContext accessor to the underlying
	// EvalContext. The CRITICAL mapping is SourceIP() → DirectRemoteIP() per
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
