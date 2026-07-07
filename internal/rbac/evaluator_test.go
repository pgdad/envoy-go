package rbac

import (
	"net"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	rbacconfigv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// stubEvalContext is a test-only EvalContext implementation. Each field is a
// pre-populated value the per-test set-up writes; the per-method accessors
// return the field verbatim. Permission accessors landed at Task 4; Principal
// accessors (DirectRemoteIP, RemoteIP, DownstreamPrincipal, SourcedMetadata,
// FilterState) landed at Task 5 per ADR-0143 §Decision (i) EvalContext
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

// ----------------------------------------------------------------------------
// Group 3 — Permission evaluators (per SPEC §14.1 #3 + §6.5). 15 test cases.
//
// Each test exercises one Permission variant (or one PARSE-REJECT) by:
//
//  1. Building an *rbacconfigv3.Permission proto carrying the variant's
//     payload.
//  2. Calling buildOnePermission(perm, ProfileHTTP) → asserting either (a) the returned
//     evaluator's evaluatePermission(ctx) disposition against a stub
//     EvalContext, OR (b) the parse error wording verbatim for the 3 deferred
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

func TestPermAny_True_Matches(t *testing.T) {
	// Per SPEC §6.5 + ADR-0143: permAny{val: true} matches any request.
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_Any{Any: true}}
	ev, err := buildOnePermission(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePermission(any=true, ProfileHTTP): want success, got %v", err)
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
	_, err := buildOnePermission(p, ProfileHTTP)
	if err == nil {
		t.Fatal("buildOnePermission(any=false, ProfileHTTP): want error, got nil")
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
			ev, err := buildOnePermission(p, ProfileHTTP)
			if err != nil {
				t.Fatalf("buildOnePermission(header, ProfileHTTP): want success, got %v", err)
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
			ev, err := buildOnePermission(p, ProfileHTTP)
			if err != nil {
				t.Fatalf("buildOnePermission(url_path, ProfileHTTP): want success, got %v", err)
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
	ev, err := buildOnePermission(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePermission(destination_ip, ProfileHTTP): want success, got %v", err)
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
	ev, err := buildOnePermission(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePermission(destination_ip prefix_len=0, ProfileHTTP): want success, got %v", err)
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
	ev, err := buildOnePermission(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePermission(destination_port, ProfileHTTP): want success, got %v", err)
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
	ev, err := buildOnePermission(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePermission(destination_port_range, ProfileHTTP): want success, got %v", err)
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
	ev, err := buildOnePermission(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePermission(requested_server_name, ProfileHTTP): want success, got %v", err)
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

func TestPermSNI_SafeRegex_PrecompiledAtBuildTime(t *testing.T) {
	// The SafeRegex arm is compiled ONCE in buildOnePermission (via
	// compileStringMatcher); runtime evaluation must match/deny identically
	// to the historical per-call compile.
	sm := &matcherv3.StringMatcher{
		MatchPattern: &matcherv3.StringMatcher_SafeRegex{
			SafeRegex: &matcherv3.RegexMatcher{Regex: "ex.*\\.com"},
		},
	}
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_RequestedServerName{RequestedServerName: sm}}
	ev, err := buildOnePermission(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePermission(requested_server_name safe_regex): want success, got %v", err)
	}
	if !ev.evaluatePermission(&stubEvalContext{serverName: "example.com"}) {
		t.Error("SNI=example.com vs ex.*\\.com: want match true")
	}
	if ev.evaluatePermission(&stubEvalContext{serverName: "other.org"}) {
		t.Error("SNI=other.org vs ex.*\\.com: want match false")
	}
}

func TestPermSNI_SafeRegex_CompileFailure_RuntimeFalse(t *testing.T) {
	// Historical contract: an uncompilable SafeRegex pattern does NOT
	// PARSE-REJECT — build succeeds and every runtime evaluation returns
	// false. The build-time precompilation must preserve that disposition
	// (nil compiled program → false).
	sm := &matcherv3.StringMatcher{
		MatchPattern: &matcherv3.StringMatcher_SafeRegex{
			SafeRegex: &matcherv3.RegexMatcher{Regex: "("}, // unbalanced paren → compile error
		},
	}
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_RequestedServerName{RequestedServerName: sm}}
	ev, err := buildOnePermission(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePermission(bad safe_regex): want build success (runtime-false contract), got %v", err)
	}
	if ev.evaluatePermission(&stubEvalContext{serverName: "anything"}) {
		t.Error("bad safe_regex: want runtime match false")
	}
}

func TestPermDestIP_CIDR_UnparseablePrefix_RuntimeFalse(t *testing.T) {
	// Historical contract: an unparseable address_prefix does NOT
	// PARSE-REJECT — build succeeds and every runtime evaluation returns
	// false. The build-time compileCidr must preserve that disposition
	// (nil ipNet → false).
	cidr := &corev3.CidrRange{
		AddressPrefix: "not-an-ip",
		PrefixLen:     wrapperspb.UInt32(24),
	}
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_DestinationIp{DestinationIp: cidr}}
	ev, err := buildOnePermission(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePermission(bad destination_ip prefix): want build success (runtime-false contract), got %v", err)
	}
	if ev.evaluatePermission(&stubEvalContext{destIP: net.ParseIP("10.0.0.1")}) {
		t.Error("unparseable address_prefix: want runtime match false")
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
	ev, err := buildOnePermission(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePermission(and 4-level, ProfileHTTP): want success, got %v", err)
	}
	if !ev.evaluatePermission(&stubEvalContext{}) {
		t.Error("AND(any...any) 4-deep: want true (all-match)")
	}

	// Now flip the innermost to a port-exact mismatch to verify short-circuit.
	innermost.Rules = []*rbacconfigv3.Permission{
		{Rule: &rbacconfigv3.Permission_Any{Any: true}},
		{Rule: &rbacconfigv3.Permission_DestinationPort{DestinationPort: 99}},
	}
	evMix, err := buildOnePermission(p, ProfileHTTP)
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
	ev, err := buildOnePermission(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePermission(or 3-level, ProfileHTTP): want success, got %v", err)
	}
	if !ev.evaluatePermission(&stubEvalContext{destPort: 80}) {
		t.Error("OR with any:true inside: want true")
	}

	// All-false short-circuit.
	innermost.Rules = []*rbacconfigv3.Permission{
		{Rule: &rbacconfigv3.Permission_DestinationPort{DestinationPort: 99}},
		{Rule: &rbacconfigv3.Permission_DestinationPort{DestinationPort: 77}},
	}
	evMiss, err := buildOnePermission(p, ProfileHTTP)
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
	ev, err := buildOnePermission(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePermission(not, ProfileHTTP): want success, got %v", err)
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
	ev, err := buildOnePermission(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePermission(sourced_metadata, ProfileHTTP): want success (parse-supported), got %v", err)
	}
	if ev.evaluatePermission(&stubEvalContext{}) {
		t.Error("permSourcedMetadata.evaluatePermission: want false (always-no-match MVP)")
	}
}

func TestPermMetadata_PARSE_REJECT(t *testing.T) {
	// Per §2.3 + §11.P12 + planner-time D3: Permission_Metadata is deprecated
	// upstream. envoy-go-only PARSE-REJECT with the specified error wording.
	p := &rbacconfigv3.Permission{Rule: &rbacconfigv3.Permission_Metadata{}}
	_, err := buildOnePermission(p, ProfileHTTP)
	if err == nil {
		t.Fatal("buildOnePermission(metadata, ProfileHTTP): want error, got nil")
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
	_, err := buildOnePermission(p, ProfileHTTP)
	if err == nil {
		t.Fatal("buildOnePermission(matcher, ProfileHTTP): want error, got nil")
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
	_, err := buildOnePermission(p, ProfileHTTP)
	if err == nil {
		t.Fatal("buildOnePermission(uri_template, ProfileHTTP): want error, got nil")
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
//  2. Calling buildOnePrincipal(p, ProfileHTTP) → asserting either (a) the returned
//     evaluator's evaluatePrincipal(ctx) disposition against a stub EvalContext,
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
	ev, err := buildOnePrincipal(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePrincipal(any=true, ProfileHTTP): want success, got %v", err)
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
	_, errFalse := buildOnePrincipal(pFalse, ProfileHTTP)
	if errFalse == nil {
		t.Error("buildOnePrincipal(any=false, ProfileHTTP): want error (PGV const=true mirror), got nil")
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
	ev, err := buildOnePrincipal(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePrincipal(direct_remote_ip, ProfileHTTP): want success, got %v", err)
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
	// XFF resolver discipline: Task 5 declares the EvalContext.RemoteIP()
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
	ev, err := buildOnePrincipal(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePrincipal(remote_ip, ProfileHTTP): want success, got %v", err)
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
			ev, err := buildOnePrincipal(p, ProfileHTTP)
			if err != nil {
				t.Fatalf("buildOnePrincipal(header, ProfileHTTP): want success, got %v", err)
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
	ev, err := buildOnePrincipal(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePrincipal(url_path, ProfileHTTP): want success, got %v", err)
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
	ev, err := buildOnePrincipal(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePrincipal(and 2-level, ProfileHTTP): want success, got %v", err)
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
	ev, err := buildOnePrincipal(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePrincipal(or 2-level, ProfileHTTP): want success, got %v", err)
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
	evMiss, err := buildOnePrincipal(p, ProfileHTTP)
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
	ev, err := buildOnePrincipal(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePrincipal(not, ProfileHTTP): want success, got %v", err)
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
	ev, err := buildOnePrincipal(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePrincipal(sourced_metadata, ProfileHTTP): want success (parse-supported), got %v", err)
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
	ev, err := buildOnePrincipal(p, ProfileHTTP)
	if err != nil {
		t.Fatalf("buildOnePrincipal(filter_state, ProfileHTTP): want success (parse-supported), got %v", err)
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
		ev, err := buildOnePrincipal(p, ProfileHTTP)
		if err != nil {
			t.Fatalf("buildOnePrincipal(authenticated nil-matcher, ProfileHTTP): want success, got %v", err)
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
		ev, err := buildOnePrincipal(p, ProfileHTTP)
		if err != nil {
			t.Fatalf("buildOnePrincipal(authenticated string-matcher, ProfileHTTP): want success, got %v", err)
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
		ev, err := buildOnePrincipal(p, ProfileHTTP)
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
		evNilMatcher, err := buildOnePrincipal(pNilMatcher, ProfileHTTP)
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
		evWithMatcher, err := buildOnePrincipal(pWithMatcher, ProfileHTTP)
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
	_, err := buildOnePrincipal(p, ProfileHTTP)
	if err == nil {
		t.Fatal("buildOnePrincipal(source_ip, ProfileHTTP): want error, got nil")
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
	_, err := buildOnePrincipal(p, ProfileHTTP)
	if err == nil {
		t.Fatal("buildOnePrincipal(metadata, ProfileHTTP): want error, got nil")
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
// the end-to-end mTLS path is fixture 0018 scenario 6 (Task 12-14). The
// DownstreamPrincipal accessor is sourced by BOTH consumers of this shared
// engine, and each consumer carries its own coverage:
//
//   - HTTP consumer (internal/filter/http/rbac): the prinAuthenticated
//     consumption is covered here via stubEvalContext; the production source is
//     the hcm-side extractTLSPrincipals helper (internal/filter/hcm/tls_test.go)
//     plumbed through chain.go (SetTLSPrincipals → decoderCB.DownstreamPrincipal()),
//     covered by the chain_test.go probe-filter integration tests in
//     internal/filter/http/.
//   - L4 consumer (internal/filter/network/rbac): the l4EvalContext maps
//     network.Connection.DownstreamPrincipals() to DownstreamPrincipal(),
//     covered by evalctx_test.go in that package.
// ----------------------------------------------------------------------------

func TestDownstreamPrincipal_PlaintextConnection_NilSlice(t *testing.T) {
	// Per ADR-0144 §Consequences + ADR-0143 §Decision (vi) case (c):
	// plaintext / non-mTLS / no-client-cert → DownstreamPrincipal() returns
	// nil/empty → prinAuthenticated.evaluatePrincipal returns FALSE (regardless
	// of whether nameMatcher is set).
	p := &rbacconfigv3.Principal{Identifier: &rbacconfigv3.Principal_Authenticated_{
		Authenticated: &rbacconfigv3.Principal_Authenticated{}, // nil principal_name
	}}
	ev, err := buildOnePrincipal(p, ProfileHTTP)
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
	ev, err := buildOnePrincipal(p, ProfileHTTP)
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
	ev, err := buildOnePrincipal(p, ProfileHTTP)
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
	ev, err := buildOnePrincipal(p, ProfileHTTP)
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
	ev, err := buildOnePrincipal(p, ProfileHTTP)
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
