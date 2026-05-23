package ratelimit

// descriptors_test.go — TDD coverage for the §4 descriptor-action engine per
// phase-24.1 PLAN Task 6. Five required tests (PLAN Step 1):
//
//  1. TestDescriptors_PerAction — one sub-row per CORE action; exact {key,value}
//     per AMEND-11 (generic_key default "generic_key"; header_value_match default
//     "header_match" with expect_match true; request_headers REQUIRES config key;
//     remote_address fixed key + IP-only value; destination_cluster fixed key +
//     supplied cluster name).
//  2. TestDescriptors_EmptyActionDrop — the TWO §4.5 behaviors:
//     (a) action returns false ⇒ WHOLE descriptor dropped, loop breaks
//         (request_headers + !skip_if_absent on missing header).
//     (b) action returns "empty-key skip" ⇒ entry dropped, descriptor survives
//         (request_headers + skip_if_absent on missing header alongside a valid
//         generic_key action).
//  3. TestDescriptors_AxisA_EarlyReturn — when the route has non-empty
//     rate_limits, only the route is walked; the vhost is NOT walked (per
//     D-RL6 — at 24.1 the route policy IS the embedded list).
//  4. TestDescriptors_OverrideDefault_VhostWalk — the OVERRIDE-default §4.3
//     behavior: route empty ⇒ vhost walked; route non-empty ⇒ vhost skipped.
//  5. TestDescriptors_EntriesActionOrder — entries[i].key matches actions[i]
//     order (AMEND-6 proto-number-faithful within a descriptor).

import (
	"net"
	"net/http"
	"testing"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/ratelimit/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// protoDescriptor is a local alias for *ratelimitv3.RateLimitDescriptor that
// keeps the proto import path bound to ONE site in this test file. The engine's
// real return type is []*ratelimitv3.RateLimitDescriptor; we re-project it
// through projectFromProto for table-readable assertions.
type protoDescriptor = ratelimitv3.RateLimitDescriptor

// ----------------------------------------------------------------------------
// Tiny builders — one per CORE action, returning a *routev3.RateLimit policy
// with a single action. Each builder is a single-statement composition for
// table-driven legibility (mirrors the compiled_config_test.go helper style).
// ----------------------------------------------------------------------------

func policyGenericKey(key, value string) *routev3.RateLimit {
	return &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{{
		ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
			GenericKey: &routev3.RateLimit_Action_GenericKey{
				DescriptorKey:   key,
				DescriptorValue: value,
			},
		},
	}}}
}

func policyRequestHeaders(headerName, descKey string, skipIfAbsent bool) *routev3.RateLimit {
	return &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{{
		ActionSpecifier: &routev3.RateLimit_Action_RequestHeaders_{
			RequestHeaders: &routev3.RateLimit_Action_RequestHeaders{
				HeaderName:    headerName,
				DescriptorKey: descKey,
				SkipIfAbsent:  skipIfAbsent,
			},
		},
	}}}
}

func policyRemoteAddress() *routev3.RateLimit {
	return &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{{
		ActionSpecifier: &routev3.RateLimit_Action_RemoteAddress_{
			RemoteAddress: &routev3.RateLimit_Action_RemoteAddress{},
		},
	}}}
}

func policyDestinationCluster() *routev3.RateLimit {
	return &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{{
		ActionSpecifier: &routev3.RateLimit_Action_DestinationCluster_{
			DestinationCluster: &routev3.RateLimit_Action_DestinationCluster{},
		},
	}}}
}

// policyHeaderValueMatch builds a single-action policy whose match list contains
// ONE present-match matcher on headerName. expectMatch nil ⇒ default true.
func policyHeaderValueMatch(descKey, descValue, headerName string, expectMatch *bool) *routev3.RateLimit {
	var em *wrapperspb.BoolValue
	if expectMatch != nil {
		em = wrapperspb.Bool(*expectMatch)
	}
	return &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{{
		ActionSpecifier: &routev3.RateLimit_Action_HeaderValueMatch_{
			HeaderValueMatch: &routev3.RateLimit_Action_HeaderValueMatch{
				DescriptorKey:   descKey,
				DescriptorValue: descValue,
				ExpectMatch:     em,
				Headers: []*routev3.HeaderMatcher{{
					Name: headerName,
					HeaderMatchSpecifier: &routev3.HeaderMatcher_PresentMatch{
						PresentMatch: true,
					},
				}},
			},
		},
	}}}
}

// addrFromIP returns a *net.TCPAddr (the canonical net.Addr concrete type
// HCM dispatch seeds via the ADR-0165 set-once primitive) for tests that
// exercise the remote_address action.
func addrFromIP(ip string) net.Addr {
	return &net.TCPAddr{IP: net.ParseIP(ip), Port: 56789}
}

// kv is a tiny test-only carrier for "descriptor entry expectation" rows.
type kv struct {
	key   string
	value string
}

// assertDescriptorEntries fails the test if the descriptor's entries don't
// match the expected (key, value) sequence exactly (order + count + values).
func assertDescriptorEntries(t *testing.T, got []kv, want []kv) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("entries: got %d, want %d (got=%v want=%v)", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("entries[%d]: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// flattenDescriptors projects []*ratelimitv3.RateLimitDescriptor into a slice-
// of-slices of (key,value) entries for terse table-driven equality.
func flattenDescriptors(ds []descriptorTestProjection) [][]kv {
	out := make([][]kv, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.entries)
	}
	return out
}

// descriptorTestProjection captures one descriptor as a slice of (key,value)
// entries, for table assertions independent of the proto pointer arithmetic.
type descriptorTestProjection struct {
	entries []kv
}

// projectDescriptors flattens []*ratelimitv3.RateLimitDescriptor into
// []descriptorTestProjection for terse equality assertions.
func projectDescriptors(ds []*descriptorEntriesForTest) []descriptorTestProjection {
	out := make([]descriptorTestProjection, 0, len(ds))
	for _, d := range ds {
		entries := make([]kv, 0, len(d.entries))
		for _, e := range d.entries {
			entries = append(entries, kv{key: e.key, value: e.value})
		}
		out = append(out, descriptorTestProjection{entries: entries})
	}
	return out
}

// descriptorEntriesForTest + entry-for-test let the test code keep its
// assertions on a stable shape without leaking the proto types into the
// per-row tables (and lets us avoid an import-only-for-test for the proto).
type descriptorEntriesForTest struct {
	entries []entryForTest
}
type entryForTest struct {
	key   string
	value string
}

// projectFromProto pulls (key, value) entries out of the production-side
// *ratelimitv3.RateLimitDescriptor return shape into the test-internal
// carrier. Single source of proto-arithmetic in this test file.
func projectFromProto(ds []*protoDescriptor) []*descriptorEntriesForTest {
	out := make([]*descriptorEntriesForTest, 0, len(ds))
	for _, d := range ds {
		entries := make([]entryForTest, 0, len(d.GetEntries()))
		for _, e := range d.GetEntries() {
			entries = append(entries, entryForTest{key: e.GetKey(), value: e.GetValue()})
		}
		out = append(out, &descriptorEntriesForTest{entries: entries})
	}
	return out
}

// ----------------------------------------------------------------------------
// Test 1: TestDescriptors_PerAction — one row per CORE action (5 rows).
// ----------------------------------------------------------------------------

func TestDescriptors_PerAction(t *testing.T) {
	const (
		downstreamIP = "203.0.113.42"
		clusterName  = "upstream-cluster"
	)
	addr := addrFromIP(downstreamIP)

	// Per AMEND-11: generic_key default key "generic_key"; header_value_match
	// default key "header_match" + expect_match default true; request_headers
	// REQUIRES a configured descriptor_key (no default); remote_address fixed
	// key "remote_address"; destination_cluster fixed key "destination_cluster".
	tests := []struct {
		name      string
		policy    *routev3.RateLimit
		headers   http.Header
		wantOne   bool   // expect exactly one descriptor (true) or zero (false)
		wantKey   string // expected entry key when wantOne
		wantValue string // expected entry value when wantOne
	}{
		{
			name:      "generic_key_default_key",
			policy:    policyGenericKey("", "static-bucket"), // empty config key ⇒ default "generic_key"
			headers:   http.Header{},
			wantOne:   true,
			wantKey:   "generic_key",
			wantValue: "static-bucket",
		},
		{
			name:      "generic_key_configured_key",
			policy:    policyGenericKey("my_bucket", "static-bucket"),
			headers:   http.Header{},
			wantOne:   true,
			wantKey:   "my_bucket",
			wantValue: "static-bucket",
		},
		{
			name: "request_headers_with_configured_key_and_present_header",
			policy: policyRequestHeaders(
				"x-user-id", "user_id", false,
			),
			headers:   http.Header{"X-User-Id": []string{"alice"}},
			wantOne:   true,
			wantKey:   "user_id",
			wantValue: "alice",
		},
		{
			name:      "remote_address_fixed_key_ip_string_value",
			policy:    policyRemoteAddress(),
			headers:   http.Header{},
			wantOne:   true,
			wantKey:   "remote_address",
			wantValue: downstreamIP,
		},
		{
			name:      "destination_cluster_fixed_key_routed_cluster_value",
			policy:    policyDestinationCluster(),
			headers:   http.Header{},
			wantOne:   true,
			wantKey:   "destination_cluster",
			wantValue: clusterName,
		},
		{
			name:      "header_value_match_default_key_expect_match_true_default",
			policy:    policyHeaderValueMatch("", "vip-bucket", "x-vip", nil),
			headers:   http.Header{"X-Vip": []string{"y"}},
			wantOne:   true,
			wantKey:   "header_match",
			wantValue: "vip-bucket",
		},
		{
			name:      "header_value_match_configured_key",
			policy:    policyHeaderValueMatch("special_match", "vip-bucket", "x-vip", nil),
			headers:   http.Header{"X-Vip": []string{"y"}},
			wantOne:   true,
			wantKey:   "special_match",
			wantValue: "vip-bucket",
		},
		{
			name: "header_value_match_expect_match_false_when_headers_absent_emits",
			policy: policyHeaderValueMatch(
				"", "no-vip-bucket", "x-vip", boolPtr(false),
			),
			headers:   http.Header{}, // x-vip absent ⇒ headers match is false; expect_match=false ⇒ emit
			wantOne:   true,
			wantKey:   "header_match",
			wantValue: "no-vip-bucket",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := projectFromProto(buildDescriptors(
				[]*routev3.RateLimit{tc.policy},
				nil, // no vhost
				tc.headers,
				addr,
				clusterName,
			))
			if !tc.wantOne {
				if len(got) != 0 {
					t.Fatalf("expected 0 descriptors, got %d", len(got))
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("expected 1 descriptor, got %d", len(got))
			}
			projection := projectDescriptors(got)
			assertDescriptorEntries(t, projection[0].entries, []kv{{key: tc.wantKey, value: tc.wantValue}})
		})
	}
}

func boolPtr(b bool) *bool { return &b }

// ----------------------------------------------------------------------------
// Test 2: TestDescriptors_EmptyActionDrop — the TWO §4.5 behaviors.
// ----------------------------------------------------------------------------

func TestDescriptors_EmptyActionDrop(t *testing.T) {
	addr := addrFromIP("198.51.100.7")
	const clusterName = "cluster-X"

	t.Run("ActionReturnsFalse_DropsWholeDescriptor_AndLoopBreaks", func(t *testing.T) {
		// A policy with two actions: (1) generic_key — fine; (2) request_headers
		// with skip_if_absent=false on a header that is ABSENT — the action
		// returns false; the entire descriptor is dropped, even though action
		// (1) would have produced a valid entry.
		policy := &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{
			{
				ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
					GenericKey: &routev3.RateLimit_Action_GenericKey{
						DescriptorValue: "first-entry-survives-not",
					},
				},
			},
			{
				ActionSpecifier: &routev3.RateLimit_Action_RequestHeaders_{
					RequestHeaders: &routev3.RateLimit_Action_RequestHeaders{
						HeaderName:    "x-required-header",
						DescriptorKey: "required",
						SkipIfAbsent:  false, // header absent + !skip_if_absent ⇒ drop descriptor
					},
				},
			},
		}}
		got := projectFromProto(buildDescriptors(
			[]*routev3.RateLimit{policy},
			nil,
			http.Header{}, // x-required-header absent
			addr,
			clusterName,
		))
		if len(got) != 0 {
			t.Fatalf("expected 0 descriptors (action returns false ⇒ drop whole), got %d: %+v", len(got), projectDescriptors(got))
		}
	})

	t.Run("ActionReturnsTrueButEmptyKey_SkipsEntry_DescriptorSurvives", func(t *testing.T) {
		// A policy with two actions: (1) generic_key — produces an entry;
		// (2) request_headers with skip_if_absent=true on an ABSENT header — the
		// action returns true but produces NO entry (empty-key skip); the
		// descriptor survives with action (1)'s entry only.
		policy := &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{
			{
				ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
					GenericKey: &routev3.RateLimit_Action_GenericKey{
						DescriptorValue: "kept",
					},
				},
			},
			{
				ActionSpecifier: &routev3.RateLimit_Action_RequestHeaders_{
					RequestHeaders: &routev3.RateLimit_Action_RequestHeaders{
						HeaderName:    "x-optional-header",
						DescriptorKey: "optional",
						SkipIfAbsent:  true, // header absent + skip_if_absent ⇒ skip ONE entry
					},
				},
			},
		}}
		got := projectFromProto(buildDescriptors(
			[]*routev3.RateLimit{policy},
			nil,
			http.Header{}, // x-optional-header absent
			addr,
			clusterName,
		))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor (skip ONE entry; descriptor survives), got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "generic_key", value: "kept"}})
	})
}

// ----------------------------------------------------------------------------
// Test 3: TestDescriptors_AxisA_EarlyReturn — per D-RL6: at 24.1 the route
// rate_limits[] IS the Axis-A embedded list; when it is non-empty, only it is
// walked (the vhost is NOT walked).
// ----------------------------------------------------------------------------

func TestDescriptors_AxisA_EarlyReturn(t *testing.T) {
	addr := addrFromIP("192.0.2.1")
	const clusterName = "cluster-X"

	route := []*routev3.RateLimit{
		policyGenericKey("route_only", "route_value"),
	}
	vhost := []*routev3.RateLimit{
		policyGenericKey("vhost_only", "vhost_value"),
	}

	got := projectFromProto(buildDescriptors(route, vhost, http.Header{}, addr, clusterName))
	if len(got) != 1 {
		t.Fatalf("expected 1 descriptor (route walked only), got %d", len(got))
	}
	proj := projectDescriptors(got)
	assertDescriptorEntries(t, proj[0].entries, []kv{{key: "route_only", value: "route_value"}})

	// The vhost-only key must NOT appear anywhere.
	for _, d := range proj {
		for _, e := range d.entries {
			if e.key == "vhost_only" {
				t.Fatalf("vhost_only key leaked through Axis-A early-return: %+v", proj)
			}
		}
	}
}

// ----------------------------------------------------------------------------
// Test 4: TestDescriptors_OverrideDefault_VhostWalk — OVERRIDE-default §4.3:
// route always walked; vhost walked ONLY when route is empty.
// ----------------------------------------------------------------------------

func TestDescriptors_OverrideDefault_VhostWalk(t *testing.T) {
	addr := addrFromIP("192.0.2.99")
	const clusterName = "cluster-X"

	t.Run("RouteEmpty_VhostWalked", func(t *testing.T) {
		vhost := []*routev3.RateLimit{
			policyGenericKey("vh_key", "vh_value"),
		}
		got := projectFromProto(buildDescriptors(nil, vhost, http.Header{}, addr, clusterName))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor (vhost walked when route empty), got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "vh_key", value: "vh_value"}})
	})

	t.Run("RouteNonEmpty_VhostSkipped_UnderOVERRIDEDefault", func(t *testing.T) {
		route := []*routev3.RateLimit{
			policyGenericKey("rt_key", "rt_value"),
		}
		vhost := []*routev3.RateLimit{
			policyGenericKey("vh_key", "vh_value"),
		}
		got := projectFromProto(buildDescriptors(route, vhost, http.Header{}, addr, clusterName))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor (vhost skipped under OVERRIDE default when route non-empty), got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "rt_key", value: "rt_value"}})
		// Defensive: vh_key must NOT appear anywhere.
		for _, d := range proj {
			for _, e := range d.entries {
				if e.key == "vh_key" {
					t.Fatalf("vh_key leaked through OVERRIDE default vhost-skip path: %+v", proj)
				}
			}
		}
	})

	t.Run("BothEmpty_NoDescriptors", func(t *testing.T) {
		got := projectFromProto(buildDescriptors(nil, nil, http.Header{}, addr, clusterName))
		if len(got) != 0 {
			t.Fatalf("expected 0 descriptors (both empty), got %d", len(got))
		}
	})
}

// ----------------------------------------------------------------------------
// Test 5: TestDescriptors_EntriesActionOrder — entries[i].key matches
// actions[i] order within a single descriptor (AMEND-6 proto-number-faithful).
// ----------------------------------------------------------------------------

func TestDescriptors_EntriesActionOrder(t *testing.T) {
	addr := addrFromIP("198.51.100.21")
	const clusterName = "cluster-ordered"

	// Build a single policy with FIVE actions, mixed order. The descriptor
	// MUST emit entries in this exact action-list order.
	policy := &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{
		{ // action 0: destination_cluster
			ActionSpecifier: &routev3.RateLimit_Action_DestinationCluster_{
				DestinationCluster: &routev3.RateLimit_Action_DestinationCluster{},
			},
		},
		{ // action 1: generic_key
			ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
				GenericKey: &routev3.RateLimit_Action_GenericKey{
					DescriptorKey:   "bucket",
					DescriptorValue: "tier-1",
				},
			},
		},
		{ // action 2: remote_address
			ActionSpecifier: &routev3.RateLimit_Action_RemoteAddress_{
				RemoteAddress: &routev3.RateLimit_Action_RemoteAddress{},
			},
		},
		{ // action 3: request_headers (present; skip_if_absent=false)
			ActionSpecifier: &routev3.RateLimit_Action_RequestHeaders_{
				RequestHeaders: &routev3.RateLimit_Action_RequestHeaders{
					HeaderName:    "x-user",
					DescriptorKey: "user",
					SkipIfAbsent:  false,
				},
			},
		},
		{ // action 4: header_value_match (matches)
			ActionSpecifier: &routev3.RateLimit_Action_HeaderValueMatch_{
				HeaderValueMatch: &routev3.RateLimit_Action_HeaderValueMatch{
					DescriptorKey:   "match_key",
					DescriptorValue: "match_value",
					Headers: []*routev3.HeaderMatcher{{
						Name: "x-tier",
						HeaderMatchSpecifier: &routev3.HeaderMatcher_PresentMatch{
							PresentMatch: true,
						},
					}},
				},
			},
		},
	}}

	headers := http.Header{
		"X-User": []string{"alice"},
		"X-Tier": []string{"gold"},
	}
	got := projectFromProto(buildDescriptors(
		[]*routev3.RateLimit{policy},
		nil,
		headers,
		addr,
		clusterName,
	))
	if len(got) != 1 {
		t.Fatalf("expected 1 descriptor, got %d", len(got))
	}
	proj := projectDescriptors(got)
	want := []kv{
		{key: "destination_cluster", value: clusterName},
		{key: "bucket", value: "tier-1"},
		{key: "remote_address", value: "198.51.100.21"},
		{key: "user", value: "alice"},
		{key: "match_key", value: "match_value"},
	}
	assertDescriptorEntries(t, proj[0].entries, want)
}

// ----------------------------------------------------------------------------
// Additional small-coverage tests for the empty-key entry skip on generic_key
// (descriptor_value empty ⇒ value empty ⇒ skip behavior per upstream
// router_ratelimit.cc:34-36). Documents the §4.5 "empty-key skip vs whole-
// drop" boundary on the generic_key + header_value_match arms.
//
// These supplement the five mandatory tests above; failure here flags a
// regression in the per-action drop discipline beyond what TestDescriptors_PerAction
// exercises.
// ----------------------------------------------------------------------------

func TestDescriptors_GenericKey_EmptyValue_DropsDescriptor(t *testing.T) {
	// generic_key with descriptor_value empty AND no default_value (24.1 does
	// not honor default_value — that field is on the proto but the engine
	// treats descriptor_value=="" as a whole-descriptor drop per upstream
	// router_ratelimit.cc:163,166-183).
	addr := addrFromIP("192.0.2.1")
	policy := policyGenericKey("", "")
	got := projectFromProto(buildDescriptors(
		[]*routev3.RateLimit{policy}, nil, http.Header{}, addr, "cluster-X",
	))
	if len(got) != 0 {
		t.Fatalf("expected 0 descriptors (generic_key empty value drops descriptor), got %d", len(got))
	}
}

func TestDescriptors_HeaderValueMatch_HeadersMismatch_ExpectMatchTrue_Drops(t *testing.T) {
	// headers match = false; expect_match = true (default) ⇒ drop descriptor
	// (action returns false).
	addr := addrFromIP("192.0.2.1")
	policy := policyHeaderValueMatch("", "vip-bucket", "x-vip", nil) // expect_match default true
	got := projectFromProto(buildDescriptors(
		[]*routev3.RateLimit{policy}, nil, http.Header{}, // x-vip absent ⇒ headers match false
		addr, "cluster-X",
	))
	if len(got) != 0 {
		t.Fatalf("expected 0 descriptors (headers mismatch + expect_match=true ⇒ drop), got %d", len(got))
	}
}

// ----------------------------------------------------------------------------
// Multi-policy fanout — multiple route policies each produce one descriptor.
// ----------------------------------------------------------------------------

func TestDescriptors_MultiplePolicies_FanOutToMultipleDescriptors(t *testing.T) {
	addr := addrFromIP("192.0.2.42")
	route := []*routev3.RateLimit{
		policyGenericKey("key1", "value1"),
		policyGenericKey("key2", "value2"),
	}
	got := projectFromProto(buildDescriptors(route, nil, http.Header{}, addr, "cluster-X"))
	if len(got) != 2 {
		t.Fatalf("expected 2 descriptors (one per policy), got %d", len(got))
	}
	proj := projectDescriptors(got)
	flat := flattenDescriptors(proj)
	if flat[0][0] != (kv{key: "key1", value: "value1"}) {
		t.Errorf("descriptor[0]: got %+v, want {key1, value1}", flat[0][0])
	}
	if flat[1][0] != (kv{key: "key2", value: "value2"}) {
		t.Errorf("descriptor[1]: got %+v, want {key2, value2}", flat[1][0])
	}
}

// ----------------------------------------------------------------------------
// Header-matcher exact-match coverage — pins the StringMatch path that
// header_value_match relies on for non-trivial matchers. Mirrors the upstream
// matcher subset commonly seen in rate-limit configs.
// ----------------------------------------------------------------------------

func TestDescriptors_HeaderValueMatch_StringMatchExact(t *testing.T) {
	addr := addrFromIP("192.0.2.1")
	const clusterName = "cluster-X"
	// Build a header_value_match policy whose matcher is StringMatch{Exact: "gold"}.
	policy := &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{{
		ActionSpecifier: &routev3.RateLimit_Action_HeaderValueMatch_{
			HeaderValueMatch: &routev3.RateLimit_Action_HeaderValueMatch{
				DescriptorValue: "gold-bucket",
				Headers: []*routev3.HeaderMatcher{{
					Name: "x-tier",
					HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
						StringMatch: &matcherv3.StringMatcher{
							MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "gold"},
						},
					},
				}},
			},
		},
	}}}

	// Case 1: header matches "gold" ⇒ descriptor produced.
	got := projectFromProto(buildDescriptors(
		[]*routev3.RateLimit{policy}, nil,
		http.Header{"X-Tier": []string{"gold"}},
		addr, clusterName,
	))
	if len(got) != 1 {
		t.Fatalf("expected 1 descriptor (exact match hit), got %d", len(got))
	}

	// Case 2: header value differs ⇒ no descriptor.
	got2 := projectFromProto(buildDescriptors(
		[]*routev3.RateLimit{policy}, nil,
		http.Header{"X-Tier": []string{"silver"}},
		addr, clusterName,
	))
	if len(got2) != 0 {
		t.Fatalf("expected 0 descriptors (exact match miss), got %d", len(got2))
	}
}

// ----------------------------------------------------------------------------
// Defensive: empty Headers slice on header_value_match.
// Upstream: matchHeaders([]) returns true (vacuous AND-fold). So expect_match=true
// + empty headers ⇒ entry produced. expect_match=false + empty headers ⇒ drop.
// ----------------------------------------------------------------------------

func TestDescriptors_HeaderValueMatch_EmptyHeadersList_VacuouslyMatches(t *testing.T) {
	addr := addrFromIP("192.0.2.1")
	policy := &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{{
		ActionSpecifier: &routev3.RateLimit_Action_HeaderValueMatch_{
			HeaderValueMatch: &routev3.RateLimit_Action_HeaderValueMatch{
				DescriptorValue: "any-bucket",
				Headers:         nil, // empty matchers list
				// expect_match default true
			},
		},
	}}}
	got := projectFromProto(buildDescriptors(
		[]*routev3.RateLimit{policy}, nil, http.Header{}, addr, "cluster-X",
	))
	if len(got) != 1 {
		t.Fatalf("expected 1 descriptor (vacuous AND ⇒ matches), got %d", len(got))
	}
}
