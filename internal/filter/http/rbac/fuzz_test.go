package rbac

import (
	"testing"

	xdscorev3 "github.com/cncf/xds/go/xds/core/v3"
	xdsmatcherv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	rbacconfigv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	rbacv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// FuzzRBACConfigParse fuzzes arbitrary byte sequences as the typed_config Any
// Value payload to New. Asserts the contract per ADR-0018 + ADR-0140 +
// ADR-0141: New returns either (factory, nil) OR (nil, error); never panics;
// never returns (nil, nil). 20th fuzzer overall (phase 02-15 contributed 19).
//
// Seed corpus per PLAN line 67 — 8 valid seeds (rules-engine ALLOW; rules-engine
// DENY; rules-engine LOG; matcher-engine canonical-Action terminal;
// rules+shadow_rules combo; matcher+shadow_matcher combo; track_per_rule_stats
// =true; per-route TPFC wholesale-override marshaled via the outer RBAC field
// since New consumes the listener-level envelope) + 5 invalid seeds (empty
// bytes; empty rules.policies map; nil permissions array; Principal_Custom
// variant fabricated via raw bytes since the proto binding doesn't expose it
// in go-control-plane v1.32.4; non-canonical matcher terminal TypeURL).
//
// The fuzz function body is intentionally minimal — only the structural
// contract (never-both-nil; never-both-set; never-panic) is asserted. The fuzz
// engine derives further inputs from these seeds at the 30s budget per
// ADR-0018 short-mode CI policy.
func FuzzRBACConfigParse(f *testing.F) {
	// ---------------------------------------------------------------------
	// 8 valid seeds
	// ---------------------------------------------------------------------

	// Helper: a minimum-viable any-permission + any-principal policies map.
	anyPolicies := func(name string) map[string]*rbacconfigv3.Policy {
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

	// (1) rules-engine ALLOW.
	addRawSeed(f, &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action:   rbacconfigv3.RBAC_ALLOW,
			Policies: anyPolicies("p_allow"),
		},
	})

	// (2) rules-engine DENY.
	addRawSeed(f, &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action:   rbacconfigv3.RBAC_DENY,
			Policies: anyPolicies("p_deny"),
		},
	})

	// (3) rules-engine LOG (LOG always-allows + matched-policy captured per
	// §1.1 amendment 5).
	addRawSeed(f, &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action:   rbacconfigv3.RBAC_LOG,
			Policies: anyPolicies("p_log"),
		},
	})

	// (4) matcher-engine canonical-Action terminal (per §11.P3 + §2.6 +
	// ADR-0142). The terminal action TypeURL is the canonical RBAC Action
	// — anything else PARSE-REJECTs at buildCompiledMatcherEngine.
	actionAny, err := anypb.New(&rbacconfigv3.Action{
		Name:   "matcher_terminal",
		Action: rbacconfigv3.RBAC_ALLOW,
	})
	if err != nil {
		f.Fatalf("seed[4] anypb.New(Action): %v", err)
	}
	addRawSeed(f, &rbacv3.RBAC{
		Matcher: &xdsmatcherv3.Matcher{
			MatcherType: &xdsmatcherv3.Matcher_MatcherList_{
				MatcherList: &xdsmatcherv3.Matcher_MatcherList{
					Matchers: []*xdsmatcherv3.Matcher_MatcherList_FieldMatcher{
						{
							Predicate: &xdsmatcherv3.Matcher_MatcherList_Predicate{
								MatchType: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_{
									SinglePredicate: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate{
										Input: &xdscorev3.TypedExtensionConfig{
											Name: "header-input-x-user",
											TypedConfig: mustMarshalAnyForFuzz(f,
												&matcherv3.HttpRequestHeaderMatchInput{HeaderName: "x-user"}),
										},
										Matcher: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_ValueMatch{
											ValueMatch: &xdsmatcherv3.StringMatcher{
												MatchPattern: &xdsmatcherv3.StringMatcher_Exact{Exact: "alice"},
											},
										},
									},
								},
							},
							OnMatch: &xdsmatcherv3.Matcher_OnMatch{
								OnMatch: &xdsmatcherv3.Matcher_OnMatch_Action{
									Action: &xdscorev3.TypedExtensionConfig{
										Name:        "action",
										TypedConfig: actionAny,
									},
								},
							},
						},
					},
				},
			},
		},
	})

	// (5) rules + shadow_rules combo (dual-engine; shadow tick parallel to
	// primary per §1.1 amendment 2).
	addRawSeed(f, &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action:   rbacconfigv3.RBAC_ALLOW,
			Policies: anyPolicies("primary"),
		},
		ShadowRules: &rbacconfigv3.RBAC{
			Action:   rbacconfigv3.RBAC_DENY,
			Policies: anyPolicies("shadow"),
		},
		RulesStatPrefix:       "primary_ns",
		ShadowRulesStatPrefix: "shadow_ns",
	})

	// (6) matcher + shadow_matcher combo (matcher-engine on both sides).
	addRawSeed(f, &rbacv3.RBAC{
		Matcher:       buildAnyAllowMatcherForFuzz(f, "primary_match"),
		ShadowMatcher: buildAnyAllowMatcherForFuzz(f, "shadow_match"),
	})

	// (7) track_per_rule_stats=true (per-policy lazy-allocation enabled per
	// §1.1 amendment 8 + ADR-0146).
	addRawSeed(f, &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action:   rbacconfigv3.RBAC_ALLOW,
			Policies: anyPolicies("p_tracked"),
		},
		TrackPerRuleStats: true,
		RulesStatPrefix:   "tracked",
	})

	// (8) per-route TPFC wholesale-override shape. New consumes the
	// listener-level envelope; the per-route wholesale-override case is
	// structurally identical to a listener-level config carrying explicit
	// per_route_stat_prefix. The seed exercises the inner RBAC shape that
	// also lives inside RBACPerRoute.rbac per §5.1 case (b) + ADR-0125 §(xii).
	addRawSeed(f, &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action:   rbacconfigv3.RBAC_DENY,
			Policies: anyPolicies("p_route_override"),
		},
		RulesStatPrefix:       "route_override",
		ShadowRulesStatPrefix: "route_override_shadow",
	})

	// ---------------------------------------------------------------------
	// 5 invalid seeds
	// ---------------------------------------------------------------------

	// (i) empty bytes — Unmarshal succeeds to empty RBAC; both engines unset
	// is structurally VALID per rbac.pb.go:33 (wholly inactive filter). To
	// hit a true rejection path we still include this in the corpus because
	// the fuzzer's contract is (factory, nil) OR (nil, err); empty bytes
	// happen to land in the (factory, nil) branch. Kept as a seed regardless
	// — the contract holds either way.
	f.Add([]byte{})

	// (ii) empty rules.policies map → defensive validation: with action set
	// but zero policies, the engine has no policies to walk. Envoy's
	// behavior here is the no-match disposition (ALLOW-no-match → deny;
	// DENY-no-match → allow). envoy-go MVP accepts this proto shape (no
	// min_items=1 enforcement at the RBAC.policies level — the
	// PGV-mirror is per-policy). Kept as a seed for shape coverage.
	addRawSeed(f, &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action:   rbacconfigv3.RBAC_ALLOW,
			Policies: map[string]*rbacconfigv3.Policy{},
		},
	})

	// (iii) nil permissions array — defensive PGV-mirror at
	// buildCompiledRulesEngine rejects with "policy %q must have at least
	// one permission" per amendment 4.
	addRawSeed(f, &rbacv3.RBAC{
		Rules: &rbacconfigv3.RBAC{
			Action: rbacconfigv3.RBAC_ALLOW,
			Policies: map[string]*rbacconfigv3.Policy{
				"empty_perms": {
					Permissions: nil,
					Principals: []*rbacconfigv3.Principal{
						{Identifier: &rbacconfigv3.Principal_Any{Any: true}},
					},
				},
			},
		},
	})

	// (iv) Principal_Custom variant — the proto binding in
	// go-control-plane v1.32.4 does NOT expose the Principal_Custom case
	// (amendment 7 finding lands at v1.37.2); the structural PARSE-REJECT
	// is encoded in buildOnePrincipal's `default:` arm. Fabricate a raw
	// bytes seed carrying an unknown principal-side oneof tag — the
	// Unmarshal MAY treat it as unknown-field (silent) OR the
	// buildOnePrincipal default arm MAY surface a PARSE-REJECT. Either
	// way the (factory, nil) | (nil, err) contract holds.
	f.Add([]byte{0x0a, 0x06, 0x0a, 0x04, 0x12, 0x02, 0xff, 0x01}) // random-shape

	// (v) non-canonical matcher terminal TypeURL — replace the canonical
	// Action TypeURL with a different one. buildCompiledMatcherEngine
	// PARSE-REJECTs via matcher.New per §11.P3 + §2.6 + ADR-0142.
	nonCanonical, err := anypb.New(&matcherv3.StringMatcher{
		MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "not-an-action"},
	})
	if err != nil {
		f.Fatalf("seed[v] anypb.New(StringMatcher): %v", err)
	}
	addRawSeed(f, &rbacv3.RBAC{
		Matcher: &xdsmatcherv3.Matcher{
			MatcherType: &xdsmatcherv3.Matcher_MatcherList_{
				MatcherList: &xdsmatcherv3.Matcher_MatcherList{
					Matchers: []*xdsmatcherv3.Matcher_MatcherList_FieldMatcher{
						{
							Predicate: &xdsmatcherv3.Matcher_MatcherList_Predicate{
								MatchType: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_{
									SinglePredicate: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate{
										Input: &xdscorev3.TypedExtensionConfig{
											Name: "header-input-x-user",
											TypedConfig: mustMarshalAnyForFuzz(f,
												&matcherv3.HttpRequestHeaderMatchInput{HeaderName: "x-user"}),
										},
										Matcher: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_ValueMatch{
											ValueMatch: &xdsmatcherv3.StringMatcher{
												MatchPattern: &xdsmatcherv3.StringMatcher_Exact{Exact: "x"},
											},
										},
									},
								},
							},
							OnMatch: &xdsmatcherv3.Matcher_OnMatch{
								OnMatch: &xdsmatcherv3.Matcher_OnMatch_Action{
									Action: &xdscorev3.TypedExtensionConfig{
										Name:        "action",
										TypedConfig: nonCanonical, // non-canonical TypeURL
									},
								},
							},
						},
					},
				},
			},
		},
	})

	// ---------------------------------------------------------------------
	// Fuzz body: structural contract assertions only.
	// ---------------------------------------------------------------------
	f.Fuzz(func(t *testing.T, raw []byte) {
		// Empty FactoryCtx (no Stats registry) per phase-14/15 precedent:
		// this fuzzer targets the typed_config Any-unmarshal + parse pipeline,
		// not the stats-registration path. buildCompiledConfig short-circuits
		// the stats path on ctx.Stats==nil per ADR-0085 nil-tolerance.
		anyMsg := &anypb.Any{TypeUrl: TypeURL, Value: raw}
		factory, err := New(anyMsg, envoyhttp.FactoryCtx{})
		if factory == nil && err == nil {
			t.Fatalf("New returned (nil, nil) — invariant violation; len(raw)=%d", len(raw))
		}
		if factory != nil && err != nil {
			t.Fatalf("New returned (factory, err) — invariant violation: %v", err)
		}
	})
}

// addRawSeed marshals msg + adds the resulting raw bytes to the fuzzer corpus.
// Helper to reduce per-seed boilerplate.
func addRawSeed(f *testing.F, msg proto.Message) {
	f.Helper()
	raw, err := proto.Marshal(msg)
	if err != nil {
		f.Fatalf("seed marshal: %v", err)
	}
	f.Add(raw)
}

// mustMarshalAnyForFuzz wraps anypb.New + f.Fatalf for inline use.
func mustMarshalAnyForFuzz(f *testing.F, msg proto.Message) *anypb.Any {
	f.Helper()
	a, err := anypb.New(msg)
	if err != nil {
		f.Fatalf("anypb.New: %v", err)
	}
	return a
}

// buildAnyAllowMatcherForFuzz returns a minimum-viable matcher tree whose
// single FieldMatcher matches header x-user==alice and terminates with a
// canonical Action{name, ALLOW}. Used by seed (6) for the matcher+shadow_matcher
// combo.
func buildAnyAllowMatcherForFuzz(f *testing.F, name string) *xdsmatcherv3.Matcher {
	f.Helper()
	actionAny, err := anypb.New(&rbacconfigv3.Action{
		Name:   name,
		Action: rbacconfigv3.RBAC_ALLOW,
	})
	if err != nil {
		f.Fatalf("anypb.New(Action): %v", err)
	}
	return &xdsmatcherv3.Matcher{
		MatcherType: &xdsmatcherv3.Matcher_MatcherList_{
			MatcherList: &xdsmatcherv3.Matcher_MatcherList{
				Matchers: []*xdsmatcherv3.Matcher_MatcherList_FieldMatcher{
					{
						Predicate: &xdsmatcherv3.Matcher_MatcherList_Predicate{
							MatchType: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_{
								SinglePredicate: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate{
									Input: &xdscorev3.TypedExtensionConfig{
										Name: "header-input-x-user",
										TypedConfig: mustMarshalAnyForFuzz(f,
											&matcherv3.HttpRequestHeaderMatchInput{HeaderName: "x-user"}),
									},
									Matcher: &xdsmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_ValueMatch{
										ValueMatch: &xdsmatcherv3.StringMatcher{
											MatchPattern: &xdsmatcherv3.StringMatcher_Exact{Exact: "alice"},
										},
									},
								},
							},
						},
						OnMatch: &xdsmatcherv3.Matcher_OnMatch{
							OnMatch: &xdsmatcherv3.Matcher_OnMatch_Action{
								Action: &xdscorev3.TypedExtensionConfig{
									Name:        "action",
									TypedConfig: actionAny,
								},
							},
						},
					},
				},
			},
		},
	}
}
