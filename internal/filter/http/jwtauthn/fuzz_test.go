package jwtauthn

import (
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	jwt_authnv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// FuzzJwtAuthnConfigParse fuzzes arbitrary byte sequences as the typed_config
// Any.Value payload to New. Asserts the contract per ADR-0018 + ADR-0148: New
// returns either (factory, nil) OR (nil, error); never panics; never returns
// (nil, nil). 21st fuzzer overall (phase 02-16 contributed 20).
//
// Seed corpus per SPEC §14.3 — 13 seeds covering JwtAuthentication variants:
//
//	(0)  minimal valid (one LocalJwks provider + one rule)
//	(1)  provider with consumed-field surface (13 fields per amendments 2+3)
//	(2)  provider with silent-ignored fields (filter_state_rules, subjects, etc.)
//	(3)  RemoteJwks + async_fetch + retry_policy
//	(4)  all 4 extraction sources (from_headers + from_params + from_cookies + defaults)
//	(5)  claim_to_headers nested dot-notation
//	(6)  inline `requires` honored per amendment 4
//	(7)  requirement_name → requirement_map resolution
//	(8)  all 6 JwtRequirement variants + recursive nesting (RequiresAny + RequiresAll)
//	(9)  filter_state_rules set (silent-ignored at parse per amendment 1)
//	(10) bypass_cors_preflight + strip_failure_response both true
//	(11) empty JwtAuthentication (all 6 outer fields default per amendment 1)
//	(12) clear_route_cache + claim_to_headers combination
//
// The fuzz function body is intentionally minimal — only the structural
// contract (never-both-nil; never-both-set; never-panic) is asserted. The fuzz
// engine derives further inputs from these seeds at the 30s budget per
// ADR-0018 short-mode CI policy.
func FuzzJwtAuthnConfigParse(f *testing.F) {
	// Helper: minimal LocalJwks provider with a single inline JWK.
	localJwks := func(issuer string) *jwt_authnv3.JwtProvider {
		return &jwt_authnv3.JwtProvider{
			Issuer: issuer,
			JwksSourceSpecifier: &jwt_authnv3.JwtProvider_LocalJwks{
				LocalJwks: &corev3.DataSource{
					Specifier: &corev3.DataSource_InlineString{
						InlineString: `{"keys":[{"kty":"RSA","kid":"k1","alg":"RS256","use":"sig","n":"0vx7","e":"AQAB"}]}`,
					},
				},
			},
		}
	}

	// Helper: minimal RouteMatch (prefix /).
	prefixMatch := &routev3.RouteMatch{
		PathSpecifier: &routev3.RouteMatch_Prefix{Prefix: "/"},
	}

	// (0) Minimal valid: one LocalJwks provider + one rule that requires it.
	addFuzzSeed(f, &jwt_authnv3.JwtAuthentication{
		Providers: map[string]*jwt_authnv3.JwtProvider{
			"p0": localJwks("https://issuer0.example"),
		},
		Rules: []*jwt_authnv3.RequirementRule{
			{
				Match: prefixMatch,
				RequirementType: &jwt_authnv3.RequirementRule_Requires{
					Requires: &jwt_authnv3.JwtRequirement{
						RequiresType: &jwt_authnv3.JwtRequirement_ProviderName{ProviderName: "p0"},
					},
				},
			},
		},
	})

	// (1) Provider with the 13 consumed fields per §6.2 + amendments 2+3.
	addFuzzSeed(f, &jwt_authnv3.JwtAuthentication{
		Providers: map[string]*jwt_authnv3.JwtProvider{
			"p_consumed": {
				Issuer:                  "https://example.com",
				Audiences:               []string{"aud1", "aud2"},
				JwksSourceSpecifier:     localJwks("ignored-here").JwksSourceSpecifier,
				Forward:                 true,
				FromHeaders:             []*jwt_authnv3.JwtHeader{{Name: "x-jwt-token", ValuePrefix: ""}},
				FromParams:              []string{"jwt_token"},
				FromCookies:             []string{"auth"},
				ForwardPayloadHeader:    "x-jwt-payload",
				PadForwardPayloadHeader: true,
				ClaimToHeaders: []*jwt_authnv3.JwtClaimToHeader{
					{HeaderName: "x-claim-sub", ClaimName: "sub"},
				},
				ClearRouteCache:  true,
				ClockSkewSeconds: 120,
			},
		},
	})

	// (2) Provider carrying silent-ignored fields per amendment 3:
	// payload_in_metadata, header_in_metadata, failed_status_in_metadata,
	// normalize_payload_in_metadata, jwt_cache_config, subjects,
	// require_expiration, max_lifetime. Parse MUST tolerate all 8 fields.
	addFuzzSeed(f, &jwt_authnv3.JwtAuthentication{
		Providers: map[string]*jwt_authnv3.JwtProvider{
			"p_silent": {
				Issuer:                     "https://silent.example",
				JwksSourceSpecifier:        localJwks("").JwksSourceSpecifier,
				PayloadInMetadata:          "my_payload",
				HeaderInMetadata:           "my_header",
				FailedStatusInMetadata:     "my_failure",
				NormalizePayloadInMetadata: &jwt_authnv3.JwtProvider_NormalizePayload{},
				JwtCacheConfig:             &jwt_authnv3.JwtCacheConfig{JwtCacheSize: 1024},
				Subjects: &matcherv3.StringMatcher{
					MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "spiffe://"},
				},
				RequireExpiration: true,
				MaxLifetime:       &durationpb.Duration{Seconds: 3600},
			},
		},
	})

	// (3) RemoteJwks + async_fetch + retry_policy (Task 3 wires the real
	// async-fetch path; the fuzz seed exercises the parse path).
	addFuzzSeed(f, &jwt_authnv3.JwtAuthentication{
		Providers: map[string]*jwt_authnv3.JwtProvider{
			"p_remote": {
				Issuer: "https://remote.example",
				JwksSourceSpecifier: &jwt_authnv3.JwtProvider_RemoteJwks{
					RemoteJwks: &jwt_authnv3.RemoteJwks{
						HttpUri: &corev3.HttpUri{
							Uri:              "https://remote.example/.well-known/jwks.json",
							HttpUpstreamType: &corev3.HttpUri_Cluster{Cluster: "jwks_cluster"},
							Timeout:          &durationpb.Duration{Seconds: 1},
						},
						CacheDuration: &durationpb.Duration{Seconds: 600},
						AsyncFetch: &jwt_authnv3.JwksAsyncFetch{
							FastListener:          true,
							FailedRefetchDuration: &durationpb.Duration{Seconds: 1},
						},
					},
				},
			},
		},
	})

	// (4) All 4 extraction sources (default Authorization+access_token implicit;
	// explicit from_headers + from_params + from_cookies covered here).
	addFuzzSeed(f, &jwt_authnv3.JwtAuthentication{
		Providers: map[string]*jwt_authnv3.JwtProvider{
			"p_extract": {
				Issuer:              "https://extract.example",
				JwksSourceSpecifier: localJwks("").JwksSourceSpecifier,
				FromHeaders: []*jwt_authnv3.JwtHeader{
					{Name: "x-jwt-a", ValuePrefix: "Bearer "},
					{Name: "x-jwt-b", ValuePrefix: ""},
				},
				FromParams:  []string{"jwt", "token"},
				FromCookies: []string{"sess", "auth"},
			},
		},
	})

	// (5) claim_to_headers nested dot-notation per §11.P10.
	addFuzzSeed(f, &jwt_authnv3.JwtAuthentication{
		Providers: map[string]*jwt_authnv3.JwtProvider{
			"p_claims": {
				Issuer:              "https://claims.example",
				JwksSourceSpecifier: localJwks("").JwksSourceSpecifier,
				ClaimToHeaders: []*jwt_authnv3.JwtClaimToHeader{
					{HeaderName: "x-user-email", ClaimName: "user.email"},
					{HeaderName: "x-user-id", ClaimName: "user.profile.id"},
				},
			},
		},
	})

	// (6) Inline `requires` honored per amendment 4.
	addFuzzSeed(f, &jwt_authnv3.JwtAuthentication{
		Providers: map[string]*jwt_authnv3.JwtProvider{
			"p6": localJwks("https://inline.example"),
		},
		Rules: []*jwt_authnv3.RequirementRule{
			{
				Match: prefixMatch,
				RequirementType: &jwt_authnv3.RequirementRule_Requires{
					Requires: &jwt_authnv3.JwtRequirement{
						RequiresType: &jwt_authnv3.JwtRequirement_ProviderAndAudiences{
							ProviderAndAudiences: &jwt_authnv3.ProviderWithAudiences{
								ProviderName: "p6",
								Audiences:    []string{"aud-x"},
							},
						},
					},
				},
			},
		},
	})

	// (7) requirement_name → requirement_map resolution per §5.1 + §11.P12.
	addFuzzSeed(f, &jwt_authnv3.JwtAuthentication{
		Providers: map[string]*jwt_authnv3.JwtProvider{
			"p7": localJwks("https://named.example"),
		},
		RequirementMap: map[string]*jwt_authnv3.JwtRequirement{
			"named_req": {
				RequiresType: &jwt_authnv3.JwtRequirement_ProviderName{ProviderName: "p7"},
			},
		},
		Rules: []*jwt_authnv3.RequirementRule{
			{
				Match: prefixMatch,
				RequirementType: &jwt_authnv3.RequirementRule_RequirementName{
					RequirementName: "named_req",
				},
			},
		},
	})

	// (8) All 6 JwtRequirement variants + recursive nesting. A single
	// requires_all wraps a requires_any wrapping the leaf variants
	// (provider_name + provider_and_audiences + allow_missing_or_failed +
	// allow_missing) per §6.6 + §11.P10 recursion test.
	addFuzzSeed(f, &jwt_authnv3.JwtAuthentication{
		Providers: map[string]*jwt_authnv3.JwtProvider{
			"p8a": localJwks("https://a.example"),
			"p8b": localJwks("https://b.example"),
		},
		Rules: []*jwt_authnv3.RequirementRule{
			{
				Match: prefixMatch,
				RequirementType: &jwt_authnv3.RequirementRule_Requires{
					Requires: &jwt_authnv3.JwtRequirement{
						RequiresType: &jwt_authnv3.JwtRequirement_RequiresAll{
							RequiresAll: &jwt_authnv3.JwtRequirementAndList{
								Requirements: []*jwt_authnv3.JwtRequirement{
									{
										RequiresType: &jwt_authnv3.JwtRequirement_RequiresAny{
											RequiresAny: &jwt_authnv3.JwtRequirementOrList{
												Requirements: []*jwt_authnv3.JwtRequirement{
													{
														RequiresType: &jwt_authnv3.JwtRequirement_ProviderName{
															ProviderName: "p8a",
														},
													},
													{
														RequiresType: &jwt_authnv3.JwtRequirement_ProviderAndAudiences{
															ProviderAndAudiences: &jwt_authnv3.ProviderWithAudiences{
																ProviderName: "p8b",
																Audiences:    []string{"aud-y"},
															},
														},
													},
													{
														RequiresType: &jwt_authnv3.JwtRequirement_AllowMissing{
															AllowMissing: &emptypb.Empty{},
														},
													},
													{
														RequiresType: &jwt_authnv3.JwtRequirement_AllowMissingOrFailed{
															AllowMissingOrFailed: &emptypb.Empty{},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})

	// (9) filter_state_rules set (silent-ignored at parse per amendment 1 +
	// §8 deferral 12). Parse MUST succeed; the field is structurally ignored.
	addFuzzSeed(f, &jwt_authnv3.JwtAuthentication{
		FilterStateRules: &jwt_authnv3.FilterStateRule{Name: "test_state"},
	})

	// (10) bypass_cors_preflight + strip_failure_response both true.
	addFuzzSeed(f, &jwt_authnv3.JwtAuthentication{
		BypassCorsPreflight:  true,
		StripFailureResponse: true,
	})

	// (11) Empty JwtAuthentication — all 6 outer fields at proto defaults.
	// Parse MUST tolerate per amendment 1 (wholly inactive filter).
	addFuzzSeed(f, &jwt_authnv3.JwtAuthentication{})

	// (12) clear_route_cache + claim_to_headers combination (per amendment 6 +
	// SPEC §6.2 — clear_route_cache fires when any claim_to_headers entry
	// emits OR payload_in_metadata is set; this seed exercises the parse-side
	// shape).
	addFuzzSeed(f, &jwt_authnv3.JwtAuthentication{
		Providers: map[string]*jwt_authnv3.JwtProvider{
			"p_clear": {
				Issuer:              "https://clear.example",
				JwksSourceSpecifier: localJwks("").JwksSourceSpecifier,
				ClearRouteCache:     true,
				ClaimToHeaders: []*jwt_authnv3.JwtClaimToHeader{
					{HeaderName: "x-claim-iss", ClaimName: "iss"},
				},
			},
		},
	})

	// Empty bytes — Unmarshal succeeds to zero-value JwtAuthentication; parse
	// returns (factory, nil) per amendment 1. Kept for shape coverage; the
	// contract holds either way.
	f.Add([]byte{})

	// ---------------------------------------------------------------------
	// Fuzz body: structural contract assertions only.
	// ---------------------------------------------------------------------
	f.Fuzz(func(t *testing.T, raw []byte) {
		// Empty FactoryCtx (no Stats registry) per phase-14/15/16 precedent:
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

// addFuzzSeed marshals msg + adds the resulting raw bytes to the fuzzer
// corpus. Mirrors phase-16 rbac fuzzer's addRawSeed helper.
func addFuzzSeed(f *testing.F, msg proto.Message) {
	f.Helper()
	raw, err := proto.Marshal(msg)
	if err != nil {
		f.Fatalf("seed marshal: %v", err)
	}
	f.Add(raw)
}
