package rbac

import (
	"strings"
	"testing"

	configrbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
)

// minimal ALLOW engine with one policy carrying a single permission/principal.
func ruleWith(perm *configrbacv3.Permission, prin *configrbacv3.Principal) *configrbacv3.RBAC {
	return &configrbacv3.RBAC{
		Action: configrbacv3.RBAC_ALLOW,
		Policies: map[string]*configrbacv3.Policy{
			"p": {Permissions: []*configrbacv3.Permission{perm}, Principals: []*configrbacv3.Principal{prin}},
		},
	}
}

func permHeaderRule() *configrbacv3.Permission {
	return &configrbacv3.Permission{Rule: &configrbacv3.Permission_Header{Header: &routev3.HeaderMatcher{Name: "x"}}}
}
func prinAnyId() *configrbacv3.Principal {
	return &configrbacv3.Principal{Identifier: &configrbacv3.Principal_Any{Any: true}}
}
func permAnyRule() *configrbacv3.Permission {
	return &configrbacv3.Permission{Rule: &configrbacv3.Permission_Any{Any: true}}
}
func prinHeaderId() *configrbacv3.Principal {
	return &configrbacv3.Principal{Identifier: &configrbacv3.Principal_Header{Header: &routev3.HeaderMatcher{Name: "x"}}}
}

func TestProfileHTTP_PermitsHTTPOnlyArms(t *testing.T) {
	if _, err := BuildRulesEngine(ruleWith(permHeaderRule(), prinAnyId()), ProfileHTTP); err != nil {
		t.Fatalf("ProfileHTTP must permit permission.header: %v", err)
	}
	if _, err := BuildRulesEngine(ruleWith(permAnyRule(), prinHeaderId()), ProfileHTTP); err != nil {
		t.Fatalf("ProfileHTTP must permit principal.header: %v", err)
	}
}

func TestProfileL4_RejectsHTTPOnlyPermissionHeader(t *testing.T) {
	_, err := BuildRulesEngine(ruleWith(permHeaderRule(), prinAnyId()), ProfileL4)
	if err == nil {
		t.Fatal("ProfileL4 must reject permission.header at compile")
	}
}

func TestProfileL4_RejectsHTTPOnlyPrincipalHeader(t *testing.T) {
	_, err := BuildRulesEngine(ruleWith(permAnyRule(), prinHeaderId()), ProfileL4)
	if err == nil {
		t.Fatal("ProfileL4 must reject principal.header at compile")
	}
}

func TestProfileL4_PermitsL4Arms(t *testing.T) {
	// destination_port permission + any principal — both L4-evaluable.
	perm := &configrbacv3.Permission{Rule: &configrbacv3.Permission_DestinationPort{DestinationPort: 8080}}
	if _, err := BuildRulesEngine(ruleWith(perm, prinAnyId()), ProfileL4); err != nil {
		t.Fatalf("ProfileL4 must permit destination_port + any: %v", err)
	}
}

func permURLPathRule() *configrbacv3.Permission {
	return &configrbacv3.Permission{Rule: &configrbacv3.Permission_UrlPath{
		UrlPath: &matcherv3.PathMatcher{
			Rule: &matcherv3.PathMatcher_Path{
				Path: &matcherv3.StringMatcher{
					MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "/admin"},
				},
			},
		},
	}}
}

func prinURLPathId() *configrbacv3.Principal {
	return &configrbacv3.Principal{Identifier: &configrbacv3.Principal_UrlPath{
		UrlPath: &matcherv3.PathMatcher{
			Rule: &matcherv3.PathMatcher_Path{
				Path: &matcherv3.StringMatcher{
					MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "/admin"},
				},
			},
		},
	}}
}

func TestProfileL4_RejectsHTTPOnlyPermissionURLPath(t *testing.T) {
	_, err := BuildRulesEngine(ruleWith(permURLPathRule(), prinAnyId()), ProfileL4)
	if err == nil {
		t.Fatal("ProfileL4 must reject permission.url_path at compile")
	}
}

func TestProfileL4_RejectsHTTPOnlyPrincipalURLPath(t *testing.T) {
	_, err := BuildRulesEngine(ruleWith(permAnyRule(), prinURLPathId()), ProfileL4)
	if err == nil {
		t.Fatal("ProfileL4 must reject principal.url_path at compile")
	}
}

// TestProfileL4RejectWording_ByteStable pins the EXACT operator-visible reject
// wording for all four ProfileL4 HTTP-only arms (permission.header,
// permission.url_path, principal.header, principal.url_path). The L4 network
// RBAC consumer (internal/filter/network/rbac) surfaces these verbatim at boot;
// drift is an operator-visible behavior change and must be deliberate.
func TestProfileL4RejectWording_ByteStable(t *testing.T) {
	cases := []struct {
		name string
		rule *configrbacv3.RBAC
		want string
	}{
		{"permission.header", ruleWith(permHeaderRule(), prinAnyId()),
			"rbac: permission.header is HTTP-only (unsupported for L4 network RBAC)"},
		{"permission.url_path", ruleWith(permURLPathRule(), prinAnyId()),
			"rbac: permission.url_path is HTTP-only (unsupported for L4 network RBAC)"},
		{"principal.header", ruleWith(permAnyRule(), prinHeaderId()),
			"rbac: principal.header is HTTP-only (unsupported for L4 network RBAC)"},
		{"principal.url_path", ruleWith(permAnyRule(), prinURLPathId()),
			"rbac: principal.url_path is HTTP-only (unsupported for L4 network RBAC)"},
	}
	for _, tc := range cases {
		_, err := BuildRulesEngine(tc.rule, ProfileL4)
		if err == nil {
			t.Fatalf("%s: ProfileL4 must reject at compile", tc.name)
		}
		// The compiler wraps the leaf arm-reject with policy/permission-index
		// context; the load-bearing operator wording is the leaf, pinned here as
		// a suffix so the wrapping prefix may evolve without a false failure.
		if !strings.HasSuffix(err.Error(), tc.want) {
			t.Fatalf("byte-stable drift: %s\n  got: %q\n want suffix: %q", tc.name, err.Error(), tc.want)
		}
	}
}
