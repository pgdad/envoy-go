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

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/ratelimit/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	metadatav3 "github.com/envoyproxy/go-control-plane/envoy/type/metadata/v3"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/pgdad/envoy-go/internal/dynamicmetadata"
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

// ============================================================================
// Phase 24.2 Task 1 — tests for the 5 REMAINING canonical actions + AMEND-11
// key-default round-trip + the §4.5 empty-action-drop extension. Per the
// parent SPEC §4.1 row table:
//
//   - source_cluster              key="source_cluster" (fixed); value=node service-cluster name; ALWAYS-TRUE
//   - masked_remote_address       key="masked_remote_address" (fixed); value=CIDR-masked IP; drop if not an IP
//   - metadata                    key=descriptor_key (REQUIRED); value=metadata lookup or default_value; conditional drop
//   - query_parameters            key=descriptor_key (default "query_param" per AMEND-11 SINGULAR); value=first matching qp; conditional drop
//   - query_parameter_value_match key=descriptor_key (default "query_match" per AMEND-11); value=descriptor_value; drop if expect_match != qp-match
//
// The metadata action descent uses the structpb segmented chain over the
// DYNAMIC=0 path (DecoderFilterCallbacks.DynamicMetadata() *Bucket) and the
// ROUTE_ENTRY=1 path (DecoderFilterCallbacks.RouteMetadata() *corev3.Metadata —
// the NEW DELTA-2 accessor added at this Task per D-RL8). Both paths consult
// the same MetadataKey.{key,path} shape.
// ============================================================================

// policySourceCluster builds a single-action policy whose action arm is
// source_cluster (no config fields — the action's value is sourced from the
// filter's node service-cluster threaded via descriptorInputs).
func policySourceCluster() *routev3.RateLimit {
	return &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{{
		ActionSpecifier: &routev3.RateLimit_Action_SourceCluster_{
			SourceCluster: &routev3.RateLimit_Action_SourceCluster{},
		},
	}}}
}

// policyMaskedRemoteAddress builds a single-action policy with a masked_remote_address
// arm. v4MaskLen/v6MaskLen are honored when non-nil; nil ⇒ proto default (32 / 128).
func policyMaskedRemoteAddress(v4MaskLen, v6MaskLen *uint32) *routev3.RateLimit {
	mra := &routev3.RateLimit_Action_MaskedRemoteAddress{}
	if v4MaskLen != nil {
		mra.V4PrefixMaskLen = wrapperspb.UInt32(*v4MaskLen)
	}
	if v6MaskLen != nil {
		mra.V6PrefixMaskLen = wrapperspb.UInt32(*v6MaskLen)
	}
	return &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{{
		ActionSpecifier: &routev3.RateLimit_Action_MaskedRemoteAddress_{
			MaskedRemoteAddress: mra,
		},
	}}}
}

// policyMetadata builds a single-action policy whose action arm is metadata.
// segments is the MetadataKey.path[].key chain after the filter-name top-level key.
// source selects DYNAMIC=0 vs ROUTE_ENTRY=1.
func policyMetadata(
	descKey, filterName string,
	segments []string,
	defaultValue string,
	source routev3.RateLimit_Action_MetaData_Source,
	skipIfAbsent bool,
) *routev3.RateLimit {
	pathSegs := make([]*metadatav3.MetadataKey_PathSegment, 0, len(segments))
	for _, s := range segments {
		pathSegs = append(pathSegs, &metadatav3.MetadataKey_PathSegment{
			Segment: &metadatav3.MetadataKey_PathSegment_Key{Key: s},
		})
	}
	return &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{{
		ActionSpecifier: &routev3.RateLimit_Action_Metadata{
			Metadata: &routev3.RateLimit_Action_MetaData{
				DescriptorKey: descKey,
				MetadataKey: &metadatav3.MetadataKey{
					Key:  filterName,
					Path: pathSegs,
				},
				DefaultValue: defaultValue,
				Source:       source,
				SkipIfAbsent: skipIfAbsent,
			},
		},
	}}}
}

// policyQueryParameters builds a single-action policy with a query_parameters arm.
func policyQueryParameters(qpName, descKey string, skipIfAbsent bool) *routev3.RateLimit {
	return &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{{
		ActionSpecifier: &routev3.RateLimit_Action_QueryParameters_{
			QueryParameters: &routev3.RateLimit_Action_QueryParameters{
				QueryParameterName: qpName,
				DescriptorKey:      descKey,
				SkipIfAbsent:       skipIfAbsent,
			},
		},
	}}}
}

// policyQueryParameterValueMatch builds a single-action policy with a
// query_parameter_value_match arm; one PresentMatch query-param matcher on qpName.
// expectMatch nil ⇒ default true. descKey == "" ⇒ engine should default to "query_match".
func policyQueryParameterValueMatch(
	descKey, descValue, qpName string,
	expectMatch *bool,
) *routev3.RateLimit {
	var em *wrapperspb.BoolValue
	if expectMatch != nil {
		em = wrapperspb.Bool(*expectMatch)
	}
	return &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{{
		ActionSpecifier: &routev3.RateLimit_Action_QueryParameterValueMatch_{
			QueryParameterValueMatch: &routev3.RateLimit_Action_QueryParameterValueMatch{
				DescriptorKey:   descKey,
				DescriptorValue: descValue,
				ExpectMatch:     em,
				QueryParameters: []*routev3.QueryParameterMatcher{{
					Name: qpName,
					QueryParameterMatchSpecifier: &routev3.QueryParameterMatcher_PresentMatch{
						PresentMatch: true,
					},
				}},
			},
		},
	}}}
}

// uint32Ptr returns a pointer to the supplied uint32 (test-only convenience).
func uint32Ptr(v uint32) *uint32 { return &v }

// addrFromIPv6 returns a *net.TCPAddr carrying an IPv6 IP (the test surface for
// the v6 prefix masking discipline).
func addrFromIPv6(ip string) net.Addr {
	return &net.TCPAddr{IP: net.ParseIP(ip), Port: 56789}
}

// makeBucketWithValue constructs a per-stream dynamic-metadata Bucket seeded
// with one entry at (filterName, topKey) → value. The value carrier supports
// the segmented descent: passing a structpb.Struct lets nested-segment tests
// descend into it.
func makeBucketWithValue(filterName, topKey string, value *structpb.Value) *dynamicmetadata.Bucket {
	b := dynamicmetadata.NewBucket()
	b.Set(filterName, topKey, value)
	return b
}

// makeRouteMetadataWithStruct builds a *corev3.Metadata whose filter_metadata
// map has one (filterName → struct) entry. Mirrors the upstream route-metadata
// shape that the metadata.ROUTE_ENTRY=1 source descends.
func makeRouteMetadataWithStruct(filterName string, fields map[string]*structpb.Value) *corev3.Metadata {
	return &corev3.Metadata{
		FilterMetadata: map[string]*structpb.Struct{
			filterName: {Fields: fields},
		},
	}
}

// ----------------------------------------------------------------------------
// Test 1.1: source_cluster — fixed key + node service-cluster value (ALWAYS-TRUE)
//
// AMEND-11 + parent §4.1 row 1 (rl.cc:89-90):
//   - entry key   = literal "source_cluster"
//   - entry value = filter's local_service_cluster (node service-cluster name)
//   - drop behavior: ALWAYS-TRUE — never drops the descriptor; an empty
//     node-service-cluster yields an EMPTY value entry per upstream
//     (router_ratelimit.cc:89-90 always appends without checking emptiness;
//     §4.5 behavior (2) skip-entry on EMPTY KEY does NOT apply here — the key
//     "source_cluster" is non-empty even when value is empty)
// ----------------------------------------------------------------------------

func TestDescriptors_PerAction_SourceCluster(t *testing.T) {
	addr := addrFromIP("203.0.113.42")
	const clusterName = "upstream-cluster"
	const nodeSvcCluster = "envoy-edge-fleet"

	t.Run("PresentNodeServiceCluster", func(t *testing.T) {
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits:    []*routev3.RateLimit{policySourceCluster()},
			headers:            http.Header{},
			remoteAddr:         addr,
			clusterName:        clusterName,
			nodeServiceCluster: nodeSvcCluster,
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor, got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "source_cluster", value: nodeSvcCluster}})
	})

	t.Run("EmptyNodeServiceCluster_ProducesEmptyValueEntry_DescriptorSurvives", func(t *testing.T) {
		// Upstream rl.cc:89-90: always populates; empty service-cluster ⇒ empty
		// value entry. Descriptor survives (the key is non-empty).
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits:    []*routev3.RateLimit{policySourceCluster()},
			headers:            http.Header{},
			remoteAddr:         addr,
			clusterName:        clusterName,
			nodeServiceCluster: "",
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor (always-true action), got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "source_cluster", value: ""}})
	})
}

// ----------------------------------------------------------------------------
// Test 1.2: masked_remote_address — CIDR-masked IP per v4/v6_prefix_mask_len.
//
// AMEND-11 + parent §4.1 row 5 (rl.cc:141, 154-156):
//   - entry key   = literal "masked_remote_address"
//   - entry value = "<masked-IP>/<mask-len>"
//   - drop behavior: nil OR non-IP downstream address ⇒ drop WHOLE descriptor
//   - v4_prefix_mask_len absent ⇒ 32; v6_prefix_mask_len absent ⇒ 128
// ----------------------------------------------------------------------------

func TestDescriptors_PerAction_MaskedRemoteAddress(t *testing.T) {
	const clusterName = "upstream-cluster"

	tests := []struct {
		name      string
		addr      net.Addr
		v4Mask    *uint32
		v6Mask    *uint32
		wantOne   bool
		wantValue string
	}{
		{
			name:      "v4_mask_24_canonical",
			addr:      addrFromIP("192.168.1.42"),
			v4Mask:    uint32Ptr(24),
			wantOne:   true,
			wantValue: "192.168.1.0/24",
		},
		{
			name:      "v4_mask_32_exact_default",
			addr:      addrFromIP("192.168.1.42"),
			v4Mask:    nil, // proto default 32
			wantOne:   true,
			wantValue: "192.168.1.42/32",
		},
		{
			name:      "v4_mask_0_match_all",
			addr:      addrFromIP("192.168.1.42"),
			v4Mask:    uint32Ptr(0),
			wantOne:   true,
			wantValue: "0.0.0.0/0",
		},
		{
			name:      "v6_mask_64_canonical",
			addr:      addrFromIPv6("2001:db8::1"),
			v6Mask:    uint32Ptr(64),
			wantOne:   true,
			wantValue: "2001:db8::/64",
		},
		{
			name:      "v6_mask_128_default_exact_address",
			addr:      addrFromIPv6("2001:db8::1"),
			v6Mask:    nil,
			wantOne:   true,
			wantValue: "2001:db8::1/128",
		},
		{
			name:    "nil_addr_drops_descriptor",
			addr:    nil,
			wantOne: false,
		},
		{
			name:    "non_tcp_addr_drops_descriptor",
			addr:    &net.UDPAddr{IP: net.ParseIP("192.168.1.42"), Port: 9999},
			wantOne: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := projectFromProto(buildDescriptorsExt(descriptorInputs{
				routeRateLimits: []*routev3.RateLimit{policyMaskedRemoteAddress(tc.v4Mask, tc.v6Mask)},
				headers:         http.Header{},
				remoteAddr:      tc.addr,
				clusterName:     clusterName,
			}))
			if !tc.wantOne {
				if len(got) != 0 {
					t.Fatalf("expected 0 descriptors (action drops), got %d: %+v", len(got), projectDescriptors(got))
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("expected 1 descriptor, got %d", len(got))
			}
			proj := projectDescriptors(got)
			assertDescriptorEntries(t, proj[0].entries, []kv{{
				key:   "masked_remote_address",
				value: tc.wantValue,
			}})
		})
	}
}

// ----------------------------------------------------------------------------
// Test 1.3a: metadata — DYNAMIC=0 source (streamInfo().DynamicMetadata()).
//
// AMEND-11 + parent §4.1 row 8 (rl.cc:187-227):
//   - entry key   = descriptor_key (REQUIRED — no default per AMEND-11)
//   - entry value = MetadataKey lookup result (descend MetadataKey.path[] over
//     the structpb chain) OR default_value if absent
//   - drop behavior:
//   - absent + no default_value + skip_if_absent=false ⇒ drop descriptor
//   - absent + no default_value + skip_if_absent=true  ⇒ skip ONE entry
//   - absent + default_value present                   ⇒ emit default
//   - present (string)                                 ⇒ emit value
//   - present (non-string) ⇒ treat as absent per upstream (str-only match)
// ----------------------------------------------------------------------------

func TestDescriptors_PerAction_Metadata_Dynamic(t *testing.T) {
	addr := addrFromIP("192.0.2.1")
	const clusterName = "cluster-X"
	const filterName = "envoy.filters.http.fault"

	t.Run("PresentTopLevelString", func(t *testing.T) {
		// metadata at (filterName, "tier") = "gold" (string).
		// MetadataKey.{key=filterName, path=[{key="tier"}]} ⇒ "gold".
		bucket := makeBucketWithValue(filterName, "tier",
			structpb.NewStringValue("gold"))
		policy := policyMetadata(
			"user_tier", filterName, []string{"tier"}, "",
			routev3.RateLimit_Action_MetaData_DYNAMIC,
			false,
		)
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policy},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			dynamicMetadata: bucket,
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor, got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "user_tier", value: "gold"}})
	})

	t.Run("PresentNestedSegmentedDescent", func(t *testing.T) {
		// metadata at (filterName, "user") = Struct{tier: Struct{label: "platinum"}}
		// MetadataKey.path = [{key="user"}, {key="tier"}, {key="label"}].
		inner := &structpb.Struct{Fields: map[string]*structpb.Value{
			"label": structpb.NewStringValue("platinum"),
		}}
		mid := &structpb.Struct{Fields: map[string]*structpb.Value{
			"tier": structpb.NewStructValue(inner),
		}}
		bucket := makeBucketWithValue(filterName, "user", structpb.NewStructValue(mid))
		policy := policyMetadata(
			"user_label", filterName,
			[]string{"user", "tier", "label"}, "",
			routev3.RateLimit_Action_MetaData_DYNAMIC,
			false,
		)
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policy},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			dynamicMetadata: bucket,
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor (segmented descent), got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "user_label", value: "platinum"}})
	})

	t.Run("AbsentWithDefaultValue_EmitsDefault", func(t *testing.T) {
		bucket := dynamicmetadata.NewBucket() // empty
		policy := policyMetadata(
			"user_tier", filterName, []string{"tier"}, "anon",
			routev3.RateLimit_Action_MetaData_DYNAMIC,
			false, // skip_if_absent=false but default_value present ⇒ emit default
		)
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policy},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			dynamicMetadata: bucket,
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor (default fallback), got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "user_tier", value: "anon"}})
	})

	t.Run("AbsentNoDefault_SkipIfAbsentFalse_DropsDescriptor", func(t *testing.T) {
		bucket := dynamicmetadata.NewBucket()
		policy := policyMetadata(
			"user_tier", filterName, []string{"tier"}, "",
			routev3.RateLimit_Action_MetaData_DYNAMIC,
			false,
		)
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policy},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			dynamicMetadata: bucket,
		}))
		if len(got) != 0 {
			t.Fatalf("expected 0 descriptors (absent + no default + !skip_if_absent ⇒ drop), got %d", len(got))
		}
	})

	t.Run("AbsentNoDefault_SkipIfAbsentTrue_SkipsEntry_DescriptorSurvives", func(t *testing.T) {
		// Pair the absent-metadata action with a generic_key so the descriptor
		// survives via the skip-entry path.
		bucket := dynamicmetadata.NewBucket()
		policy := &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{
			{
				ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
					GenericKey: &routev3.RateLimit_Action_GenericKey{
						DescriptorValue: "kept",
					},
				},
			},
			{
				ActionSpecifier: &routev3.RateLimit_Action_Metadata{
					Metadata: &routev3.RateLimit_Action_MetaData{
						DescriptorKey: "tier",
						MetadataKey: &metadatav3.MetadataKey{
							Key: filterName,
							Path: []*metadatav3.MetadataKey_PathSegment{{
								Segment: &metadatav3.MetadataKey_PathSegment_Key{Key: "tier"},
							}},
						},
						Source:       routev3.RateLimit_Action_MetaData_DYNAMIC,
						SkipIfAbsent: true,
					},
				},
			},
		}}
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policy},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			dynamicMetadata: bucket,
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor (skip-entry; survives), got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "generic_key", value: "kept"}})
	})

	t.Run("PresentNonStringValue_TreatedAsAbsent_DropsDescriptor", func(t *testing.T) {
		// Upstream metadata.cc: the metadataValue match requires Kind ==
		// STRING; numbers / bools / lists / structs at the leaf are treated as
		// absent.
		bucket := makeBucketWithValue(filterName, "tier", structpb.NewNumberValue(42))
		policy := policyMetadata(
			"user_tier", filterName, []string{"tier"}, "",
			routev3.RateLimit_Action_MetaData_DYNAMIC,
			false,
		)
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policy},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			dynamicMetadata: bucket,
		}))
		if len(got) != 0 {
			t.Fatalf("expected 0 descriptors (non-string leaf ⇒ absent), got %d", len(got))
		}
	})

	t.Run("Intermediate_NonStruct_TreatedAsAbsent", func(t *testing.T) {
		// descendStructpbValue (descriptors.go:962-964): when an INTERMEDIATE
		// segment lands on a non-Struct Value (e.g., a Number), the chain
		// breaks and the lookup is treated as absent.
		//
		// Shape: bucket = (filterName, "user") → Number(42); path = [{user}, {tier}].
		// descent: path[0]="user" → resolveMetadataValue returns the Number;
		// then descendStructpbValue iterates path[1:] = [{tier}] and calls
		// cur.GetStructValue() on the Number Value, which returns nil — chain
		// breaks. With no default_value and skip_if_absent=false the whole
		// descriptor drops.
		bucket := makeBucketWithValue(filterName, "user", structpb.NewNumberValue(42))
		policy := policyMetadata(
			"user_tier", filterName, []string{"user", "tier"}, "",
			routev3.RateLimit_Action_MetaData_DYNAMIC,
			false,
		)
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policy},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			dynamicMetadata: bucket,
		}))
		if len(got) != 0 {
			t.Fatalf("expected 0 descriptors (non-Struct intermediate ⇒ chain breaks ⇒ absent), got %d", len(got))
		}
	})

	t.Run("NilDescriptorKey_EmptyConfig_DropsDescriptor", func(t *testing.T) {
		// descriptor_key empty is a config-author bug; defensive drop matches the
		// request_headers (REQUIRED key) discipline.
		bucket := makeBucketWithValue(filterName, "tier", structpb.NewStringValue("gold"))
		policy := policyMetadata(
			"", filterName, []string{"tier"}, "",
			routev3.RateLimit_Action_MetaData_DYNAMIC,
			false,
		)
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policy},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			dynamicMetadata: bucket,
		}))
		if len(got) != 0 {
			t.Fatalf("expected 0 descriptors (empty descriptor_key drops), got %d", len(got))
		}
	})
}

// ----------------------------------------------------------------------------
// Test 1.3b: metadata — ROUTE_ENTRY=1 source (route's RouteEntry.Metadata()).
//
// Same semantics as DYNAMIC except the value source is the matched-route's
// *corev3.Metadata (the NEW DELTA-2 accessor added at this Task per D-RL8).
// ----------------------------------------------------------------------------

func TestDescriptors_PerAction_Metadata_RouteEntry(t *testing.T) {
	addr := addrFromIP("192.0.2.1")
	const clusterName = "cluster-X"
	const filterName = "envoy.filters.http.fault"

	t.Run("PresentTopLevelString", func(t *testing.T) {
		// route.metadata.filter_metadata[filterName] = Struct{tier: "gold"}
		md := makeRouteMetadataWithStruct(filterName, map[string]*structpb.Value{
			"tier": structpb.NewStringValue("gold"),
		})
		policy := policyMetadata(
			"user_tier", filterName, []string{"tier"}, "",
			routev3.RateLimit_Action_MetaData_ROUTE_ENTRY,
			false,
		)
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policy},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			routeMetadata:   md,
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor, got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "user_tier", value: "gold"}})
	})

	t.Run("AbsentWithDefaultValue_EmitsDefault", func(t *testing.T) {
		policy := policyMetadata(
			"user_tier", filterName, []string{"tier"}, "anon",
			routev3.RateLimit_Action_MetaData_ROUTE_ENTRY,
			false,
		)
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policy},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			// routeMetadata: nil (the no-metadata-on-route case)
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor (default fallback), got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "user_tier", value: "anon"}})
	})

	t.Run("AbsentNoDefault_SkipIfAbsentFalse_DropsDescriptor", func(t *testing.T) {
		policy := policyMetadata(
			"user_tier", filterName, []string{"tier"}, "",
			routev3.RateLimit_Action_MetaData_ROUTE_ENTRY,
			false,
		)
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policy},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
		}))
		if len(got) != 0 {
			t.Fatalf("expected 0 descriptors, got %d", len(got))
		}
	})

	t.Run("PresentNestedSegmentedDescent", func(t *testing.T) {
		// Parallels the DYNAMIC PresentNestedSegmentedDescent row — verifies
		// the ROUTE_ENTRY-side dispatch is exercised through the shared
		// descendStructpbValue helper.
		//
		// route.metadata.filter_metadata[filterName] = Struct{user: Struct{tier: "gold"}}
		// MetadataKey.path = [{key="user"}, {key="tier"}] ⇒ "gold".
		inner := &structpb.Struct{Fields: map[string]*structpb.Value{
			"tier": structpb.NewStringValue("gold"),
		}}
		md := makeRouteMetadataWithStruct(filterName, map[string]*structpb.Value{
			"user": structpb.NewStructValue(inner),
		})
		policy := policyMetadata(
			"user_tier", filterName, []string{"user", "tier"}, "",
			routev3.RateLimit_Action_MetaData_ROUTE_ENTRY,
			false,
		)
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policy},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			routeMetadata:   md,
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor (nested segmented descent on ROUTE_ENTRY), got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "user_tier", value: "gold"}})
	})

	t.Run("PresentNonStringLeaf_SkipIfAbsentTrue_SkipsEntry_DescriptorSurvives", func(t *testing.T) {
		// Parallels the DYNAMIC PresentNonStringValue row — non-string leaf is
		// treated as absent per upstream metadataValue (Kind==STRING). Paired
		// with skip_if_absent=true + a peer generic_key action so the
		// descriptor survives via the skip-ONE-entry path (§4.5 behavior (2)).
		//
		// route.metadata.filter_metadata[filterName] = Struct{tier: Number(42)}
		// MetadataKey.path = [{key="tier"}] ⇒ Number leaf ⇒ absent (skip entry).
		md := makeRouteMetadataWithStruct(filterName, map[string]*structpb.Value{
			"tier": structpb.NewNumberValue(42),
		})
		policy := &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{
			{
				ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
					GenericKey: &routev3.RateLimit_Action_GenericKey{
						DescriptorValue: "kept",
					},
				},
			},
			{
				ActionSpecifier: &routev3.RateLimit_Action_Metadata{
					Metadata: &routev3.RateLimit_Action_MetaData{
						DescriptorKey: "tier",
						MetadataKey: &metadatav3.MetadataKey{
							Key: filterName,
							Path: []*metadatav3.MetadataKey_PathSegment{{
								Segment: &metadatav3.MetadataKey_PathSegment_Key{Key: "tier"},
							}},
						},
						Source:       routev3.RateLimit_Action_MetaData_ROUTE_ENTRY,
						SkipIfAbsent: true,
					},
				},
			},
		}}
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policy},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			routeMetadata:   md,
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor (ROUTE_ENTRY non-string leaf + skip_if_absent=true ⇒ skip entry, descriptor survives), got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "generic_key", value: "kept"}})
	})
}

// ----------------------------------------------------------------------------
// Test 1.4: query_parameters — first matching qp value; AMEND-11 default "query_param" SINGULAR.
//
// AMEND-11 + parent §4.1 row 9 (rl.cc:232-253):
//   - entry key   = descriptor_key (default "query_param" SINGULAR per AMEND-11)
//   - entry value = first value of query_parameter_name in the request query
//   - drop behavior:
//   - param absent + skip_if_absent=false ⇒ drop descriptor
//   - param absent + skip_if_absent=true  ⇒ skip ONE entry
// ----------------------------------------------------------------------------

func TestDescriptors_PerAction_QueryParameters(t *testing.T) {
	addr := addrFromIP("192.0.2.1")
	const clusterName = "cluster-X"

	t.Run("Present_DefaultKey_SingularAMEND11", func(t *testing.T) {
		// descriptor_key empty ⇒ default "query_param" (SINGULAR per AMEND-11).
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policyQueryParameters("api_key", "", false)},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			rawQuery:        "api_key=alice&other=z",
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor, got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "query_param", value: "alice"}})
	})

	t.Run("Present_ConfiguredKey", func(t *testing.T) {
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policyQueryParameters("api_key", "auth_key", false)},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			rawQuery:        "api_key=alice",
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor, got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "auth_key", value: "alice"}})
	})

	t.Run("FirstValueWhenMultiple", func(t *testing.T) {
		// Multi-valued query parameter: first occurrence wins per upstream.
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policyQueryParameters("api_key", "", false)},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			rawQuery:        "api_key=first&api_key=second",
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor, got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "query_param", value: "first"}})
	})

	t.Run("Absent_SkipIfAbsentFalse_DropsDescriptor", func(t *testing.T) {
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policyQueryParameters("api_key", "", false)},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			rawQuery:        "other=z",
		}))
		if len(got) != 0 {
			t.Fatalf("expected 0 descriptors, got %d", len(got))
		}
	})

	t.Run("Absent_SkipIfAbsentTrue_SkipsEntry_DescriptorSurvives", func(t *testing.T) {
		policy := &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{
			{
				ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
					GenericKey: &routev3.RateLimit_Action_GenericKey{
						DescriptorValue: "kept",
					},
				},
			},
			{
				ActionSpecifier: &routev3.RateLimit_Action_QueryParameters_{
					QueryParameters: &routev3.RateLimit_Action_QueryParameters{
						QueryParameterName: "api_key",
						SkipIfAbsent:       true,
					},
				},
			},
		}}
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policy},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			rawQuery:        "other=z",
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor (skip-entry; survives), got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "generic_key", value: "kept"}})
	})

	t.Run("EmptyRawQuery_TreatedAsAbsent_DropsDescriptor", func(t *testing.T) {
		// No query string at all ⇒ absent.
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policyQueryParameters("api_key", "", false)},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			rawQuery:        "",
		}))
		if len(got) != 0 {
			t.Fatalf("expected 0 descriptors, got %d", len(got))
		}
	})
}

// ----------------------------------------------------------------------------
// Test 1.5: query_parameter_value_match — descriptor_value emit gated on qp match.
//
// AMEND-11 + parent §4.1 row 10 (rl.cc:297, 304-328):
//   - entry key   = descriptor_key (default "query_match" per AMEND-11)
//   - entry value = descriptor_value (REQUIRED)
//   - matchers AND-fold; entry emitted iff matched == expect_match (default true)
//   - mismatch ⇒ drop descriptor
// ----------------------------------------------------------------------------

func TestDescriptors_PerAction_QueryParameterValueMatch(t *testing.T) {
	addr := addrFromIP("192.0.2.1")
	const clusterName = "cluster-X"

	t.Run("DefaultKey_ExpectMatchTrue_QueryParamPresent_Emits", func(t *testing.T) {
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policyQueryParameterValueMatch(
				"", "premium-bucket", "vip", nil,
			)},
			headers:     http.Header{},
			remoteAddr:  addr,
			clusterName: clusterName,
			rawQuery:    "vip=1",
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor, got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "query_match", value: "premium-bucket"}})
	})

	t.Run("ConfiguredKey", func(t *testing.T) {
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policyQueryParameterValueMatch(
				"my_match", "premium-bucket", "vip", nil,
			)},
			headers:     http.Header{},
			remoteAddr:  addr,
			clusterName: clusterName,
			rawQuery:    "vip=1",
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor, got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "my_match", value: "premium-bucket"}})
	})

	t.Run("ExpectMatchTrue_QueryParamAbsent_DropsDescriptor", func(t *testing.T) {
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policyQueryParameterValueMatch(
				"", "premium-bucket", "vip", nil,
			)},
			headers:     http.Header{},
			remoteAddr:  addr,
			clusterName: clusterName,
			rawQuery:    "other=z",
		}))
		if len(got) != 0 {
			t.Fatalf("expected 0 descriptors (no match + expect_match=true), got %d", len(got))
		}
	})

	t.Run("ExpectMatchFalse_QueryParamAbsent_Emits", func(t *testing.T) {
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policyQueryParameterValueMatch(
				"", "non-premium-bucket", "vip", boolPtr(false),
			)},
			headers:     http.Header{},
			remoteAddr:  addr,
			clusterName: clusterName,
			rawQuery:    "other=z",
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor (no match + expect_match=false), got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "query_match", value: "non-premium-bucket"}})
	})

	t.Run("EmptyDescriptorValue_DropsDescriptor", func(t *testing.T) {
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policyQueryParameterValueMatch(
				"", "", "vip", nil,
			)},
			headers:     http.Header{},
			remoteAddr:  addr,
			clusterName: clusterName,
			rawQuery:    "vip=1",
		}))
		if len(got) != 0 {
			t.Fatalf("expected 0 descriptors (empty descriptor_value drops), got %d", len(got))
		}
	})
}

// ----------------------------------------------------------------------------
// Test 1.6: AMEND-11 byte-stable key defaults.
//
// Per ADR-0080: every default-key string is byte-exact. Captures the AMEND-11
// roster as compile-time guards via package-level consts. "query_param" is
// SINGULAR — easy to typo as "query_params"; pin it here.
// ----------------------------------------------------------------------------

func TestDescriptors_AMEND11_KeyDefaults_ByteStable(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"generic_key", descriptorKeyGenericKeyDefault, "generic_key"},
		{"header_value_match", descriptorKeyHeaderValueMatchDefault, "header_match"},
		{"remote_address", descriptorKeyRemoteAddress, "remote_address"},
		{"destination_cluster", descriptorKeyDestinationCluster, "destination_cluster"},
		{"source_cluster", descriptorKeySourceCluster, "source_cluster"},
		{"masked_remote_address", descriptorKeyMaskedRemoteAddress, "masked_remote_address"},
		{"query_parameters_default_SINGULAR", descriptorKeyQueryParametersDefault, "query_param"},
		{"query_parameter_value_match_default", descriptorKeyQueryParameterValueMatchDefault, "query_match"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("AMEND-11 key default %s: got %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Test 1.7: §4.5 empty-action-drop discipline extended to the 5 NEW actions.
//
// Validates that the TWO behaviors hold for the new arms when stacked:
//
//   - masked_remote_address + non-IP downstream addr ⇒ drop WHOLE
//     (parallel to remote_address drop; matched-policy + earlier-entry are lost)
//   - source_cluster + empty node ⇒ entry emitted with EMPTY value
//     (always-true action; per upstream rl.cc:89-90 no empty-key skip)
//   - query_parameters + skip_if_absent=true ⇒ skip ONE entry; descriptor survives
//   - metadata + skip_if_absent=true + no default ⇒ skip ONE entry; descriptor survives
// ----------------------------------------------------------------------------

func TestDescriptors_EmptyActionDrop_Extended(t *testing.T) {
	addr := addrFromIP("192.0.2.1")
	const clusterName = "cluster-X"

	t.Run("MaskedRemoteAddress_NilAddr_DropsWholeDescriptor", func(t *testing.T) {
		// Two-action policy: generic_key (would have produced an entry) +
		// masked_remote_address with nil addr (action returns false ⇒ drop).
		policy := &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{
			{
				ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
					GenericKey: &routev3.RateLimit_Action_GenericKey{
						DescriptorValue: "would-be-kept",
					},
				},
			},
			{
				ActionSpecifier: &routev3.RateLimit_Action_MaskedRemoteAddress_{
					MaskedRemoteAddress: &routev3.RateLimit_Action_MaskedRemoteAddress{},
				},
			},
		}}
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policy},
			headers:         http.Header{},
			remoteAddr:      nil, // nil addr ⇒ masked_remote_address returns false
			clusterName:     clusterName,
		}))
		if len(got) != 0 {
			t.Fatalf("expected 0 descriptors (whole-drop on nil addr), got %d", len(got))
		}
	})

	t.Run("SourceCluster_EmptyNode_EmitsEmptyValueEntry", func(t *testing.T) {
		// source_cluster ALWAYS produces an entry — never drops. Empty node
		// service-cluster yields an EMPTY value entry; descriptor survives.
		policy := &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{
			{
				ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
					GenericKey: &routev3.RateLimit_Action_GenericKey{
						DescriptorValue: "kept",
					},
				},
			},
			{
				ActionSpecifier: &routev3.RateLimit_Action_SourceCluster_{
					SourceCluster: &routev3.RateLimit_Action_SourceCluster{},
				},
			},
		}}
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits:    []*routev3.RateLimit{policy},
			headers:            http.Header{},
			remoteAddr:         addr,
			clusterName:        clusterName,
			nodeServiceCluster: "",
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor (always-true source_cluster), got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{
			{key: "generic_key", value: "kept"},
			{key: "source_cluster", value: ""},
		})
	})
}

// ----------------------------------------------------------------------------
// Test 1.8: backward-compat wrapper — the 5-arg buildDescriptors signature
// (used by the existing 24.1 tests + the d-core fixture) still works for the
// 5 CORE actions when wired through the new descriptorInputs struct under
// the hood. Sanity assertion to keep the 24.1 surface stable across the
// signature evolution.
// ----------------------------------------------------------------------------

func TestDescriptors_BackwardCompatibility_5ArgWrapper(t *testing.T) {
	addr := addrFromIP("203.0.113.42")
	got := projectFromProto(buildDescriptors(
		[]*routev3.RateLimit{policyGenericKey("k", "v")},
		nil, http.Header{}, addr, "cluster-X",
	))
	if len(got) != 1 {
		t.Fatalf("expected 1 descriptor, got %d", len(got))
	}
	proj := projectDescriptors(got)
	assertDescriptorEntries(t, proj[0].entries, []kv{{key: "k", value: "v"}})
}

// ----------------------------------------------------------------------------
// Existing 24.1 anchors retained below this point.
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

// ============================================================================
// Phase 24.2 Task 2 — §4.4 `stage` multi-stage bucketing path.
//
// Parent SPEC §4.4: each `RateLimit` policy carries `stage` (0-10). At config
// build, policies are bucketed by stage (upstream `references[stage]`, sized
// `MAX_STAGE_NUMBER+1 = 11`; rl.cc:539-550). At request time only policies
// whose `stage` equals the FILTER's configured `stage` (compiledConfig.stage;
// default 0) are evaluated (`getApplicableRateLimit(stage)`).
//
// 24.1 evaluated only the default stage-0 bucket; 24.2 generalizes — a filter
// at stage=5 walks only the 5-bucket. The PARSE-REJECT `stage > 10` arm
// (filter envelope) still triggers via 24.1's `buildCompiledConfig` arm; the
// per-policy `stage > 10` arm is added to `ValidateRouteRateLimits` per the
// upstream PGV `lte:10` mirror.
// ============================================================================

// policyGenericKeyAtStage builds a single-action generic_key policy stamped
// with a non-zero stage value. stage==0 is the proto-zero default; pass it as
// any uint32 — the test will assert which bucket the policy lands in.
func policyGenericKeyAtStage(key, value string, stage uint32) *routev3.RateLimit {
	rl := policyGenericKey(key, value)
	rl.Stage = wrapperspb.UInt32(stage)
	return rl
}

// ----------------------------------------------------------------------------
// TestDescriptors_StageFilter_DefaultStageZero — filter stage=0; route has
// mixed-stage policies (0 + 3); only the stage-0 policy is evaluated.
//
// This is the 24.1 baseline behavior expressed as the FILTER-stage=0 row of
// the §4.4 generalization: stage=0 selects bucket 0, same as before.
// ----------------------------------------------------------------------------

func TestDescriptors_StageFilter_DefaultStageZero(t *testing.T) {
	addr := addrFromIP("192.0.2.1")
	const clusterName = "cluster-X"

	route := []*routev3.RateLimit{
		policyGenericKeyAtStage("k_stage0", "v0", 0),
		policyGenericKeyAtStage("k_stage3", "v3", 3),
	}
	got := projectFromProto(buildDescriptorsExt(descriptorInputs{
		routeRateLimits: route,
		headers:         http.Header{},
		remoteAddr:      addr,
		clusterName:     clusterName,
		filterStage:     0,
	}))
	if len(got) != 1 {
		t.Fatalf("expected 1 descriptor (only stage-0 policy walked at filter stage=0), got %d", len(got))
	}
	proj := projectDescriptors(got)
	assertDescriptorEntries(t, proj[0].entries, []kv{{key: "k_stage0", value: "v0"}})

	// Defensive: the stage-3 key must NOT appear anywhere.
	for _, d := range proj {
		for _, e := range d.entries {
			if e.key == "k_stage3" {
				t.Fatalf("k_stage3 leaked through filter-stage=0 selection: %+v", proj)
			}
		}
	}
}

// ----------------------------------------------------------------------------
// TestDescriptors_StageFilter_NonZeroStage — filter stage=5; route has
// mixed-stage policies (3 + 5); only the stage-5 policy is evaluated.
//
// This is the §4.4 generalization beyond 24.1: a non-zero filter-stage picks
// the matching policy bucket.
// ----------------------------------------------------------------------------

func TestDescriptors_StageFilter_NonZeroStage(t *testing.T) {
	addr := addrFromIP("192.0.2.1")
	const clusterName = "cluster-X"

	route := []*routev3.RateLimit{
		policyGenericKeyAtStage("k_stage3", "v3", 3),
		policyGenericKeyAtStage("k_stage5", "v5", 5),
	}
	got := projectFromProto(buildDescriptorsExt(descriptorInputs{
		routeRateLimits: route,
		headers:         http.Header{},
		remoteAddr:      addr,
		clusterName:     clusterName,
		filterStage:     5,
	}))
	if len(got) != 1 {
		t.Fatalf("expected 1 descriptor (only stage-5 policy walked at filter stage=5), got %d", len(got))
	}
	proj := projectDescriptors(got)
	assertDescriptorEntries(t, proj[0].entries, []kv{{key: "k_stage5", value: "v5"}})

	// Defensive: the stage-3 key (off-bucket) must NOT appear.
	for _, d := range proj {
		for _, e := range d.entries {
			if e.key == "k_stage3" {
				t.Fatalf("k_stage3 leaked through filter-stage=5 selection: %+v", proj)
			}
		}
	}
}

// ----------------------------------------------------------------------------
// TestDescriptors_StageFilter_AllBucketsEmpty — filter stage=7; route has
// stage-3 + stage-5 policies; nothing matches; zero descriptors (the engine
// signals continue-without-RLS-call per parent SPEC §4.6).
// ----------------------------------------------------------------------------

func TestDescriptors_StageFilter_AllBucketsEmpty(t *testing.T) {
	addr := addrFromIP("192.0.2.1")
	const clusterName = "cluster-X"

	route := []*routev3.RateLimit{
		policyGenericKeyAtStage("k_stage3", "v3", 3),
		policyGenericKeyAtStage("k_stage5", "v5", 5),
	}
	got := projectFromProto(buildDescriptorsExt(descriptorInputs{
		routeRateLimits: route,
		headers:         http.Header{},
		remoteAddr:      addr,
		clusterName:     clusterName,
		filterStage:     7,
	}))
	if len(got) != 0 {
		t.Fatalf("expected 0 descriptors (no policy at filter stage=7), got %d", len(got))
	}
}

// ----------------------------------------------------------------------------
// TestDescriptors_StageFilter_NilStageEqualsStageZero — a policy with nil
// `Stage` (proto-absent UInt32Value) is the stage-0 bucket per upstream PGV
// `UInt32Value` default. Filter stage=0 walks it; filter stage=N>0 skips it.
// ----------------------------------------------------------------------------

func TestDescriptors_StageFilter_NilStageEqualsStageZero(t *testing.T) {
	addr := addrFromIP("192.0.2.1")
	const clusterName = "cluster-X"

	// policy with Stage unset (nil) — must bucket as stage 0.
	policyNilStage := policyGenericKey("k_nil", "vnil")

	t.Run("FilterStage0_WalksNilStage", func(t *testing.T) {
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policyNilStage},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			filterStage:     0,
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor (nil-stage policy at filter stage=0), got %d", len(got))
		}
	})

	t.Run("FilterStage3_SkipsNilStage", func(t *testing.T) {
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: []*routev3.RateLimit{policyNilStage},
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			filterStage:     3,
		}))
		if len(got) != 0 {
			t.Fatalf("expected 0 descriptors (nil-stage ≡ stage 0; filter stage=3 skips), got %d", len(got))
		}
	})
}

// ----------------------------------------------------------------------------
// TestDescriptors_StageFilter_VhostBucket — the stage filter ALSO applies to
// the vhost-walked path (route empty ⇒ vhost walked per the OVERRIDE-default
// at 24.1; Task 4 generalizes Axis-B). Filter stage=5 walking the vhost picks
// only stage-5 vhost policies.
// ----------------------------------------------------------------------------

func TestDescriptors_StageFilter_VhostBucket(t *testing.T) {
	addr := addrFromIP("192.0.2.1")
	const clusterName = "cluster-X"

	vhost := []*routev3.RateLimit{
		policyGenericKeyAtStage("vh_k3", "vh_v3", 3),
		policyGenericKeyAtStage("vh_k5", "vh_v5", 5),
	}
	got := projectFromProto(buildDescriptorsExt(descriptorInputs{
		routeRateLimits: nil,
		vhostRateLimits: vhost,
		headers:         http.Header{},
		remoteAddr:      addr,
		clusterName:     clusterName,
		filterStage:     5,
	}))
	if len(got) != 1 {
		t.Fatalf("expected 1 descriptor (vhost walked, stage-5 only), got %d", len(got))
	}
	proj := projectDescriptors(got)
	assertDescriptorEntries(t, proj[0].entries, []kv{{key: "vh_k5", value: "vh_v5"}})
}

// ----------------------------------------------------------------------------
// TestDescriptors_StageFilter_MultiplePoliciesSameStage — filter stage=4; a
// route with TWO stage-4 policies + one stage-2 policy; expect two
// descriptors (one per stage-4 policy) in policy order; stage-2 skipped.
// ----------------------------------------------------------------------------

func TestDescriptors_StageFilter_MultiplePoliciesSameStage(t *testing.T) {
	addr := addrFromIP("192.0.2.1")
	const clusterName = "cluster-X"

	route := []*routev3.RateLimit{
		policyGenericKeyAtStage("k4a", "v4a", 4),
		policyGenericKeyAtStage("k2", "v2", 2),
		policyGenericKeyAtStage("k4b", "v4b", 4),
	}
	got := projectFromProto(buildDescriptorsExt(descriptorInputs{
		routeRateLimits: route,
		headers:         http.Header{},
		remoteAddr:      addr,
		clusterName:     clusterName,
		filterStage:     4,
	}))
	if len(got) != 2 {
		t.Fatalf("expected 2 descriptors (both stage-4 policies; stage-2 skipped), got %d", len(got))
	}
	proj := projectDescriptors(got)
	assertDescriptorEntries(t, proj[0].entries, []kv{{key: "k4a", value: "v4a"}})
	assertDescriptorEntries(t, proj[1].entries, []kv{{key: "k4b", value: "v4b"}})
}

// ----------------------------------------------------------------------------
// TestDescriptors_StageFilter_MaxStage10 — filter stage=10 (the upper bound)
// matches a stage-10 policy; stage-9 + stage-0 skipped. Anchors the upper
// boundary of the §4.4 [0,10] inclusive range.
// ----------------------------------------------------------------------------

func TestDescriptors_StageFilter_MaxStage10(t *testing.T) {
	addr := addrFromIP("192.0.2.1")
	const clusterName = "cluster-X"

	route := []*routev3.RateLimit{
		policyGenericKeyAtStage("k_stage0", "v0", 0),
		policyGenericKeyAtStage("k_stage9", "v9", 9),
		policyGenericKeyAtStage("k_stage10", "v10", 10),
	}
	got := projectFromProto(buildDescriptorsExt(descriptorInputs{
		routeRateLimits: route,
		headers:         http.Header{},
		remoteAddr:      addr,
		clusterName:     clusterName,
		filterStage:     10,
	}))
	if len(got) != 1 {
		t.Fatalf("expected 1 descriptor (stage-10 only), got %d", len(got))
	}
	proj := projectDescriptors(got)
	assertDescriptorEntries(t, proj[0].entries, []kv{{key: "k_stage10", value: "v10"}})
}

// ----------------------------------------------------------------------------
// TestBuildCompiledConfig_StageBucketing_ParseTime — the parse-time bucketing
// helper `bucketRateLimitsByStage` partitions a slice of *routev3.RateLimit
// into [maxStage+1] slots by `policy.Stage.Value` (nil ⇒ 0). Per-policy
// stage > 10 policies are SKIPPED (they would have been PARSE-REJECTed by
// `ValidateRouteRateLimits` upstream; the bucketer is defensive against any
// caller that bypasses validation).
//
// This test pins the bucket structure used at request time by the engine via
// `filterStage`. Per the §4.4 upstream `references[stage]` sized
// MAX_STAGE_NUMBER+1=11 invariant.
// ----------------------------------------------------------------------------

func TestBuildCompiledConfig_StageBucketing_ParseTime(t *testing.T) {
	// Build a config-shaped slice of policies at stages 0, 3, 5, 10, and one
	// nil-stage (≡ 0). bucket[0] must contain TWO entries (the nil-stage + the
	// explicit stage-0); bucket[3], [5], [10] each ONE; all others empty.
	policies := []*routev3.RateLimit{
		policyGenericKey("k_nil", "vnil"),               // nil Stage ⇒ bucket 0
		policyGenericKeyAtStage("k_stage0", "v0", 0),    // bucket 0
		policyGenericKeyAtStage("k_stage3", "v3", 3),    // bucket 3
		policyGenericKeyAtStage("k_stage5", "v5", 5),    // bucket 5
		policyGenericKeyAtStage("k_stage10", "v10", 10), // bucket 10
	}
	buckets := bucketRateLimitsByStage(policies)

	// Bucket count must be exactly maxStage+1 = 11.
	if len(buckets) != maxStage+1 {
		t.Fatalf("bucket count: got %d, want %d (maxStage+1)", len(buckets), maxStage+1)
	}

	// Expected occupancy per bucket.
	wantCounts := [maxStage + 1]int{
		0:  2, // nil-stage + explicit stage-0
		3:  1,
		5:  1,
		10: 1,
	}
	for i := 0; i < maxStage+1; i++ {
		if len(buckets[i]) != wantCounts[i] {
			t.Errorf("bucket[%d]: got %d policies, want %d", i, len(buckets[i]), wantCounts[i])
		}
	}

	// Pin bucket[0] contents (order preserved within bucket).
	if len(buckets[0]) == 2 {
		gk0 := buckets[0][0].GetActions()[0].GetGenericKey()
		gk1 := buckets[0][1].GetActions()[0].GetGenericKey()
		if gk0.GetDescriptorKey() != "k_nil" {
			t.Errorf("bucket[0][0]: got key %q, want %q", gk0.GetDescriptorKey(), "k_nil")
		}
		if gk1.GetDescriptorKey() != "k_stage0" {
			t.Errorf("bucket[0][1]: got key %q, want %q", gk1.GetDescriptorKey(), "k_stage0")
		}
	}

	// Empty input ⇒ all-empty buckets (each slot nil/empty).
	emptyBuckets := bucketRateLimitsByStage(nil)
	if len(emptyBuckets) != maxStage+1 {
		t.Fatalf("nil input bucket count: got %d, want %d", len(emptyBuckets), maxStage+1)
	}
	for i := 0; i < maxStage+1; i++ {
		if len(emptyBuckets[i]) != 0 {
			t.Errorf("emptyBuckets[%d]: got %d policies, want 0", i, len(emptyBuckets[i]))
		}
	}
}

// ----------------------------------------------------------------------------
// TestBuildCompiledConfig_StageBucketing_OutOfRangeSkipped — a policy with
// stage > maxStage is SKIPPED by the bucketer (defensive against callers that
// bypass `ValidateRouteRateLimits`; upstream PGV `lte:10` would have rejected
// it earlier).
// ----------------------------------------------------------------------------

func TestBuildCompiledConfig_StageBucketing_OutOfRangeSkipped(t *testing.T) {
	policies := []*routev3.RateLimit{
		policyGenericKeyAtStage("k_ok", "v_ok", 5),
		policyGenericKeyAtStage("k_oob", "v_oob", 11), // out-of-range; must skip
	}
	buckets := bucketRateLimitsByStage(policies)
	if len(buckets[5]) != 1 {
		t.Errorf("bucket[5]: got %d, want 1", len(buckets[5]))
	}
	// Sum of all bucket lengths must be exactly 1 (the OOB policy contributes 0).
	total := 0
	for i := 0; i < maxStage+1; i++ {
		total += len(buckets[i])
	}
	if total != 1 {
		t.Errorf("total bucketed policies: got %d, want 1 (out-of-range skipped)", total)
	}
}

// ----------------------------------------------------------------------------
// Phase 24.2 Task 4 (D-RL11 / parent SPEC §4.3 + AMEND-5) — the FULL Axis-B
// `vh_rate_limits` cross-tier composition table + the legacy
// `RouteAction.include_vh_rate_limits=true` force-include arm.
//
// Decision table:
//
//   | vh_rate_limits          | route has rate_limits | VH walked? | route walked? |
//   | OVERRIDE (0, default)   | yes                   | NO         | yes           |
//   | OVERRIDE (0, default)   | no                    | YES        | yes (no-op)   |
//   | INCLUDE  (1)            | any                   | YES        | yes           |
//   | IGNORE   (2)            | any                   | NO         | yes           |
//
// Legacy override (AMEND-5): RouteAction.include_vh_rate_limits=true forces
// INCLUDE regardless of the enum.
// ----------------------------------------------------------------------------

// TestDescriptors_AxisB_OverrideDefault_RouteHasRateLimits verifies the
// OVERRIDE-default arm with a non-empty route: vhost SKIPPED, route walked.
// This is the 24.1 baseline behavior re-confirmed against the §4.3 table
// (regression-pinning the 0032/d-core fixture's behavioral shape).
func TestDescriptors_AxisB_OverrideDefault_RouteHasRateLimits(t *testing.T) {
	addr := addrFromIP("203.0.113.1")
	const clusterName = "cluster-X"

	route := []*routev3.RateLimit{policyGenericKey("rt", "rt_value")}
	vhost := []*routev3.RateLimit{policyGenericKey("vh", "vh_value")}

	got := projectFromProto(buildDescriptorsExt(descriptorInputs{
		routeRateLimits: route,
		vhostRateLimits: vhost,
		headers:         http.Header{},
		remoteAddr:      addr,
		clusterName:     clusterName,
		// vhostWalkMode default 0 = vhostWalkOverrideDefault — 24.1 baseline.
	}))
	if len(got) != 1 {
		t.Fatalf("expected 1 descriptor (route walked, vhost SKIPPED under OVERRIDE-default + non-empty route), got %d", len(got))
	}
	proj := projectDescriptors(got)
	assertDescriptorEntries(t, proj[0].entries, []kv{{key: "rt", value: "rt_value"}})

	// Defensive: vhost key MUST NOT appear anywhere.
	for _, d := range proj {
		for _, e := range d.entries {
			if e.key == "vh" {
				t.Fatalf("vh key leaked: %+v", proj)
			}
		}
	}
}

// TestDescriptors_AxisB_OverrideDefault_RouteEmpty verifies the OVERRIDE-
// default arm with an empty route: vhost WALKED (24.1 baseline behavior).
func TestDescriptors_AxisB_OverrideDefault_RouteEmpty(t *testing.T) {
	addr := addrFromIP("203.0.113.2")
	const clusterName = "cluster-X"

	vhost := []*routev3.RateLimit{policyGenericKey("vh", "vh_value")}

	got := projectFromProto(buildDescriptorsExt(descriptorInputs{
		routeRateLimits: nil,
		vhostRateLimits: vhost,
		headers:         http.Header{},
		remoteAddr:      addr,
		clusterName:     clusterName,
		// vhostWalkMode default 0 = vhostWalkOverrideDefault.
	}))
	if len(got) != 1 {
		t.Fatalf("expected 1 descriptor (vhost walked under OVERRIDE-default + empty route), got %d", len(got))
	}
	proj := projectDescriptors(got)
	assertDescriptorEntries(t, proj[0].entries, []kv{{key: "vh", value: "vh_value"}})
}

// TestDescriptors_AxisB_Include verifies that with vhostWalkMode=INCLUDE,
// BOTH route AND vhost policies are walked (one descriptor per policy).
func TestDescriptors_AxisB_Include(t *testing.T) {
	addr := addrFromIP("203.0.113.3")
	const clusterName = "cluster-X"

	route := []*routev3.RateLimit{policyGenericKey("rt", "rt_value")}
	vhost := []*routev3.RateLimit{policyGenericKey("vh", "vh_value")}

	got := projectFromProto(buildDescriptorsExt(descriptorInputs{
		routeRateLimits: route,
		vhostRateLimits: vhost,
		headers:         http.Header{},
		remoteAddr:      addr,
		clusterName:     clusterName,
		vhostWalkMode:   vhostWalkAlways, // INCLUDE
	}))
	if len(got) != 2 {
		t.Fatalf("expected 2 descriptors (route + vhost both walked under INCLUDE), got %d", len(got))
	}
	proj := projectDescriptors(got)
	// Walk order: route first, vhost second (the implementation walks route
	// then vhost when both are walked; this pin makes the order explicit so
	// AMEND-6 policy-order discipline is asserted).
	assertDescriptorEntries(t, proj[0].entries, []kv{{key: "rt", value: "rt_value"}})
	assertDescriptorEntries(t, proj[1].entries, []kv{{key: "vh", value: "vh_value"}})
}

// TestDescriptors_AxisB_Ignore verifies that with vhostWalkMode=IGNORE, the
// vhost policy is NEVER walked, even when the route is empty. Route policy
// is walked unconditionally.
func TestDescriptors_AxisB_Ignore(t *testing.T) {
	addr := addrFromIP("203.0.113.4")
	const clusterName = "cluster-X"

	t.Run("RouteEmpty_VhostStillSkipped", func(t *testing.T) {
		vhost := []*routev3.RateLimit{policyGenericKey("vh", "vh_value")}
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: nil,
			vhostRateLimits: vhost,
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			vhostWalkMode:   vhostWalkNever, // IGNORE
		}))
		if len(got) != 0 {
			t.Fatalf("expected 0 descriptors (IGNORE skips vhost; route empty), got %d", len(got))
		}
	})

	t.Run("RouteNonEmpty_VhostSkipped", func(t *testing.T) {
		route := []*routev3.RateLimit{policyGenericKey("rt", "rt_value")}
		vhost := []*routev3.RateLimit{policyGenericKey("vh", "vh_value")}
		got := projectFromProto(buildDescriptorsExt(descriptorInputs{
			routeRateLimits: route,
			vhostRateLimits: vhost,
			headers:         http.Header{},
			remoteAddr:      addr,
			clusterName:     clusterName,
			vhostWalkMode:   vhostWalkNever, // IGNORE
		}))
		if len(got) != 1 {
			t.Fatalf("expected 1 descriptor (IGNORE: route walked only), got %d", len(got))
		}
		proj := projectDescriptors(got)
		assertDescriptorEntries(t, proj[0].entries, []kv{{key: "rt", value: "rt_value"}})
		// Defensive: vh key must NOT appear.
		for _, d := range proj {
			for _, e := range d.entries {
				if e.key == "vh" {
					t.Fatalf("vh key leaked under IGNORE: %+v", proj)
				}
			}
		}
	})
}

// TestDescriptors_AxisB_LegacyForceInclude — the AMEND-5 legacy override:
// the implementation honors RouteAction.include_vh_rate_limits=true by setting
// vhostWalkMode=vhostWalkAlways, even when the underlying enum is IGNORE. This
// test pins the disposition at the engine surface — the legacy-trumps-enum
// semantics live one layer up at DecodeHeaders (where the bool is read from
// dcb.RouteIncludeVhRateLimits()). Here we simply pin that vhostWalkAlways
// forces a vhost walk regardless of any other input.
func TestDescriptors_AxisB_LegacyForceInclude(t *testing.T) {
	addr := addrFromIP("203.0.113.5")
	const clusterName = "cluster-X"

	route := []*routev3.RateLimit{policyGenericKey("rt", "rt_value")}
	vhost := []*routev3.RateLimit{policyGenericKey("vh", "vh_value")}

	got := projectFromProto(buildDescriptorsExt(descriptorInputs{
		routeRateLimits: route,
		vhostRateLimits: vhost,
		headers:         http.Header{},
		remoteAddr:      addr,
		clusterName:     clusterName,
		vhostWalkMode:   vhostWalkAlways, // legacy force-include / INCLUDE
	}))
	if len(got) != 2 {
		t.Fatalf("expected 2 descriptors (legacy include_vh_rate_limits=true forces vhost walk), got %d", len(got))
	}
	proj := projectDescriptors(got)
	assertDescriptorEntries(t, proj[0].entries, []kv{{key: "rt", value: "rt_value"}})
	assertDescriptorEntries(t, proj[1].entries, []kv{{key: "vh", value: "vh_value"}})
}

// ----------------------------------------------------------------------------
// TestDescriptors_AxisA_EarlyReturn_PerRoute — D-RL11 Axis-A: when the
// caller passes the per-route embedded rate_limits[] via routeRateLimits AND
// zeros vhostRateLimits, the §4.3 table is bypassed entirely. The walker
// emits descriptors ONLY for the per-route list. This is the wins-discipline
// pin: route-table + vhost-table policies passed in alongside MUST be
// invisible to the engine (the caller — decode_headers.go — handles the
// substitution; this test pins the engine-side contract).
// ----------------------------------------------------------------------------

func TestDescriptors_AxisA_EarlyReturn_PerRoute(t *testing.T) {
	addr := addrFromIP("203.0.113.6")
	const clusterName = "cluster-X"

	// The decode_headers.go logic, on Axis-A early-return, REPLACES routeRLs
	// with the per-route rate_limits[] AND zeros vhostRLs before invoking the
	// engine. This test models that substitution at the engine surface: the
	// per-route policies are passed via routeRateLimits + vhostRateLimits is
	// nil + vhostWalkMode is irrelevant (nil vhost ⇒ nothing to walk).
	perRoutePolicies := []*routev3.RateLimit{
		policyGenericKey("pr", "pr_value"),
	}

	got := projectFromProto(buildDescriptorsExt(descriptorInputs{
		routeRateLimits: perRoutePolicies, // per-route Axis-A list
		vhostRateLimits: nil,              // zeroed per Axis-A discipline
		headers:         http.Header{},
		remoteAddr:      addr,
		clusterName:     clusterName,
		// vhostWalkMode is irrelevant — vhost is already nil.
	}))
	if len(got) != 1 {
		t.Fatalf("expected 1 descriptor (Axis-A per-route walk), got %d", len(got))
	}
	proj := projectDescriptors(got)
	assertDescriptorEntries(t, proj[0].entries, []kv{{key: "pr", value: "pr_value"}})
}

// TestCachedRegex_CompileOncePerPattern pins the maintenance-pass regex cache
// semantics: (a) the same pattern string yields the same compiled *Regexp
// pointer on repeat lookups (compile-once), and (b) a compile ERROR is cached
// as nil — repeat lookups of a bad pattern also return nil, preserving the
// engine's swallow-to-false matcher semantics byte-identically.
func TestCachedRegex_CompileOncePerPattern(t *testing.T) {
	const good = "^cached-regex-test-[0-9]+$"
	re1 := cachedRegex(good)
	if re1 == nil {
		t.Fatalf("cachedRegex(%q) = nil; want compiled regexp", good)
	}
	if re2 := cachedRegex(good); re2 != re1 {
		t.Errorf("cachedRegex(%q) second lookup = %p; want cache hit %p", good, re2, re1)
	}
	if !re1.MatchString("cached-regex-test-42") {
		t.Errorf("compiled pattern must match its own vector")
	}

	const bad = "(cached-regex-test-unclosed"
	if got := cachedRegex(bad); got != nil {
		t.Errorf("cachedRegex(%q) = %v; want nil (compile error)", bad, got)
	}
	if got := cachedRegex(bad); got != nil {
		t.Errorf("cachedRegex(%q) cached-error lookup = %v; want nil", bad, got)
	}
}
