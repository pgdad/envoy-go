package ratelimit

// fuzz_test.go — 33rd project-wide fuzzer `FuzzRateLimitConfigParse` per
// phase-24.1 PLAN Task 8 + parent SPEC §6.9 + ADR-0018 baseline; extended at
// phase-24.2 PLAN Task 7 per D-RL16 (corpus extension only — no new fuzzer;
// project count stays at 33).
//
// Drives arbitrary byte sequences as the typed_config Any.Value payload
// through THREE surfaces:
//
//  1. `buildCompiledConfig` — the §5.1 PARSE-REJECT roster + AMEND-3
//     defaults/clamps body (compiled_config.go). The fuzz body uses an empty
//     `envoyhttp.FactoryCtx{}` so the cluster-load arms 10-12 fall through
//     to §5.1 arm 10 PARSE-REJECT (`parseRejectClusterManagerNotAvailable`)
//     cleanly — that arm is upstream of the cluster-manager dereference, so
//     any prior arm-firing input surfaces its own byte-stable error and any
//     deeper input gets the arm-10 PARSE-REJECT. EITHER outcome is
//     must-never-panic.
//
//  2. `buildDescriptors` + `ValidateRouteRateLimits` — the §4 engine + §5.2
//     validator (descriptors.go + compiled_config.go::ValidateRouteRateLimits).
//     The fuzz body ALSO attempts to interpret the raw bytes as a
//     `*routev3.RateLimit` proto and threads it as a single-policy slice
//     through both surfaces. This covers the case where random fuzz inputs
//     happen to decode coherently into the route-table-side proto.
//
//  3. `validatePerRouteRateLimit` + `compilePerRouteForRequest` — the 24.2
//     Task-3 per-route TPFC compile surface (compiled_perroute.go). The
//     fuzz body ALSO attempts to interpret the raw bytes as a
//     `*ratelimitfilterv3.RateLimitPerRoute` proto and drives both the
//     ADR-0110 single-chokepoint validator AND the request-time projection
//     against the random shape. Covers the case where random fuzz inputs
//     happen to decode coherently into the 10th canonical TPFC shape.
//
// # Seed corpus per PLAN Task 8 (24.1) + Task 7 (24.2) + parent §6.9
//
// 31 hand-curated `f.Add` seeds at 24.1 phase-done; extended at 24.2 Task 7
// with 15 additional seeds (46 total) covering:
//
//   - Valid full config — all 13 AMEND-3 fields populated (1 seed)
//
//   - §5.1 PARSE-REJECT arms — one seed per arm (10 seeds; arms 1, 2, 3, 4,
//     5, 6, 7, 8, 9; arm 10-12 cluster-load arms cannot be exercised through
//     a proto-shape seed since they require ctx-side state — those fire from
//     the fuzz body's empty FactoryCtx{} naturally)
//
//   - §5.2 PARSE-REJECT arms — one seed per arm via `routev3.RateLimit`
//     shape (3 seeds — disable_key non-empty / extension action /
//     dynamic_metadata action; consumed by the engine's defensive
//     belt-and-suspenders drop arm AND by ValidateRouteRateLimits when
//     interpreted as the route-table-side proto)
//
//   - CORE action exercises — one seed per action (5 seeds — generic_key /
//     request_headers / remote_address / destination_cluster /
//     header_value_match — each as a route-level rate_limits[] single-policy
//     proto for the engine surface; ALSO interpretable as filter-shape proto
//     bytes for the parse surface, where Unmarshal will succeed-with-mismatch
//     and arm-1 (domain empty) typically fires)
//
//   - Empty config — proto-zero RateLimit (1 seed — arm 1 fires)
//
//   - Boundary / edge-case neighbors — stage exactly 10 (boundary passing);
//     response_headers exactly 10 entries (boundary passing); google_grpc arm
//     set (§5.1 arm 7); envoy_grpc target_specifier absent (§5.1 arm 8);
//     timeout zero / very large; rate_limited_status 200 (< 400 clamp);
//     status_on_error 50 (< 100 clamp); status_on_error 600 (> 511 clamp);
//     enable_x_ratelimit_headers DRAFT_VERSION_03; stat_prefix populated
//     (~10 seeds)
//
//   - Raw garbage bytes — verifies proto-Unmarshal-failure path (1 seed)
//
// 24.2 Task 7 extension (15 new seeds; corpus only — no new fuzzer):
//
//   - 5 remaining §4 action arms — one seed each as a single-policy
//     `routev3.RateLimit` proto (source_cluster / masked_remote_address /
//     metadata / query_parameters / query_parameter_value_match — drives the
//     full 10-action dispatch in `buildDescriptors` for the route-shape
//     interpretation)
//
//   - 6 `RateLimitPerRoute` seeds (Surface 3 — TPFC compile):
//     · vh_rate_limits = OVERRIDE / INCLUDE / IGNORE (3 seeds — Axis-B
//       inclusion enum bounds)
//     · override_option = OVERRIDE_POLICY / INCLUDE_POLICY / IGNORE_POLICY
//       (3 seeds — AMEND-4 PARSE-ACCEPTED-but-IGNORED arm; DEFAULT=0 is the
//       proto-zero shape already covered by the OVERRIDE seed)
//
//   - Stage boundary arms — per-policy `stage=5` (new arm under 24.2 Task 2
//     multi-stage bucketing) + per-policy `stage=11` (the new Task 2
//     per-policy PARSE-REJECT arm — `ValidateRouteRateLimits` must reject)
//
//   - Per-route `domain` non-empty (AMEND-4 PARSE-ACCEPT — request-time
//     wins-discipline at Task 4)
//
//   - Legacy `RouteAction` proto bytes carrying `include_vh_rate_limits=true`
//     (the byte shape — the fuzz body cannot type-assert through a
//     RouteAction, but the bytes still exercise the proto-Unmarshal-mismatch
//     defensive paths in all 3 surfaces)
//
//   - X-RateLimit DRAFT_VERSION_03 paired with a 24.2 Task 5 emit-arm shape
//     (variant of the 24.1 seed 28 with stat_prefix populated — exercises
//     the cluster-scoped modulator + X-RateLimit toggle together)
//
// 30s runtime envelope per SPEC §14.3 + ADR-0018 short-mode CI policy.
//
// # Seed authoring strategy
//
// Uses inline `f.Add(b)` over `testdata/fuzz/<name>/` corpus files per the
// phase-21 adaptive_concurrency / phase-23 admission_control precedents —
// portable + version-controlled + no testdata-file convention. Seeds REUSE
// `validRateLimitConfig()` from `compiled_config_test.go` (intra-package
// `_test.go` helpers are visible across test files in the same package).

import (
	"net"
	"net/http"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	rlsv3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratelimitfilterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	metadatav3 "github.com/envoyproxy/go-control-plane/envoy/type/metadata/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// FuzzRateLimitConfigParse drives arbitrary byte sequences as the typed_config
// Any.Value payload through `buildCompiledConfig` (the §5.1 PARSE-REJECT
// roster + AMEND-3 defaults) AND — when the same bytes happen to unmarshal
// coherently as a `routev3.RateLimit` — through the §4 descriptor engine
// (`buildDescriptors`) + the §5.2 validator (`ValidateRouteRateLimits`). 33rd
// project-wide fuzzer per phase-24.1 PLAN Task 8 + parent SPEC §6.9.
//
// Must-never-panic across both surfaces. Tolerates Unmarshal failures (the
// random-bytes typical case) + PARSE-REJECT branches (the arm-firing case)
// without complaint — the structural contract is the no-panic invariant.
func FuzzRateLimitConfigParse(f *testing.F) {
	addSeed := func(msg proto.Message) {
		f.Helper()
		b, err := proto.Marshal(msg)
		if err != nil {
			f.Fatalf("seed marshal: %v", err)
		}
		f.Add(b)
	}

	// -------------------------------------------------------------------------
	// Seed 1: Valid full config (baseline; all 13 AMEND-3 fields populated).
	// -------------------------------------------------------------------------
	addSeed(&ratelimitfilterv3.RateLimit{
		Domain:                         "test-domain",
		Stage:                          5,
		RequestType:                    "both",
		Timeout:                        durationpb.New(30_000_000), // 30ms
		FailureModeDeny:                true,
		RateLimitedAsResourceExhausted: true,
		RateLimitService:               &rlsv3.RateLimitServiceConfig{GrpcService: validRLSGrpcService()},
		EnableXRatelimitHeaders:        ratelimitfilterv3.RateLimit_DRAFT_VERSION_03,
		DisableXEnvoyRatelimitedHeader: true,
		RateLimitedStatus:              &typev3.HttpStatus{Code: typev3.StatusCode(429)},
		StatusOnError:                  &typev3.HttpStatus{Code: typev3.StatusCode(503)},
		StatPrefix:                     "primary",
		ResponseHeadersToAdd: []*corev3.HeaderValueOption{
			{Header: &corev3.HeaderValue{Key: "x-rl-tag", Value: "v1"}},
		},
	})

	// -------------------------------------------------------------------------
	// Seeds 2-11: Each §5.1 PARSE-REJECT arm (filter-shape proto seeds).
	// -------------------------------------------------------------------------

	// Seed 2 — §5.1 Arm 1: domain empty.
	{
		c := validRateLimitConfig()
		c.Domain = ""
		addSeed(c)
	}

	// Seed 3 — §5.1 Arm 2: rate_limit_service absent.
	{
		c := validRateLimitConfig()
		c.RateLimitService = nil
		addSeed(c)
	}

	// Seed 4 — §5.1 Arm 3: stage > 10 (exactly 11).
	{
		c := validRateLimitConfig()
		c.Stage = 11
		addSeed(c)
	}

	// Seed 5 — §5.1 Arm 4: request_type not in {internal, external, both, ""}.
	{
		c := validRateLimitConfig()
		c.RequestType = "bogus"
		addSeed(c)
	}

	// Seed 6 — §5.1 Arm 5: response_headers_to_add > 10 entries (11 entries).
	{
		c := validRateLimitConfig()
		c.ResponseHeadersToAdd = make([]*corev3.HeaderValueOption, 11)
		for i := range c.ResponseHeadersToAdd {
			c.ResponseHeadersToAdd[i] = &corev3.HeaderValueOption{
				Header: &corev3.HeaderValue{Key: "x-rl", Value: "v"},
			}
		}
		addSeed(c)
	}

	// Seed 7 — §5.1 Arm 6: rate_limit_service.grpc_service absent.
	{
		c := validRateLimitConfig()
		c.RateLimitService = &rlsv3.RateLimitServiceConfig{GrpcService: nil}
		addSeed(c)
	}

	// Seed 8 — §5.1 Arm 7: google_grpc arm not supported (envoy-go-strict).
	{
		c := validRateLimitConfig()
		c.RateLimitService = &rlsv3.RateLimitServiceConfig{
			GrpcService: &corev3.GrpcService{
				TargetSpecifier: &corev3.GrpcService_GoogleGrpc_{
					GoogleGrpc: &corev3.GrpcService_GoogleGrpc{StatPrefix: "google_grpc"},
				},
			},
		}
		addSeed(c)
	}

	// Seed 9 — §5.1 Arm 8: envoy_grpc target_specifier unset (GrpcService present
	// but no oneof arm).
	{
		c := validRateLimitConfig()
		c.RateLimitService = &rlsv3.RateLimitServiceConfig{
			GrpcService: &corev3.GrpcService{},
		}
		addSeed(c)
	}

	// Seed 10 — §5.1 Arm 9: envoy_grpc.cluster_name empty.
	{
		c := validRateLimitConfig()
		c.RateLimitService = &rlsv3.RateLimitServiceConfig{
			GrpcService: &corev3.GrpcService{
				TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: ""},
				},
			},
		}
		addSeed(c)
	}

	// Seed 11 — §5.1 Arm 10/11/12 prerequisite: valid envoy_grpc with a cluster
	// name that the empty-FactoryCtx{} cluster-manager-nil arm will reject (arm
	// 10) — the same shape also surfaces in many derived inputs.
	{
		c := validRateLimitConfig()
		// (validRateLimitConfig already wires envoy_grpc + cluster name; the
		// empty FactoryCtx{} ClusterManager will trigger arm 10 PARSE-REJECT.)
		_ = c
		addSeed(c)
	}

	// -------------------------------------------------------------------------
	// Seeds 12-14: §5.2 PARSE-REJECT arms (route-shape proto seeds — the
	// fuzz body interprets these as filter-shape proto bytes which will hit
	// arm 1 / arm 4 / etc., AND attempts the route-shape interpretation for
	// the engine + ValidateRouteRateLimits surface).
	// -------------------------------------------------------------------------

	// Seed 12 — §5.2 Arm 1: route-level disable_key non-empty.
	addSeed(&routev3.RateLimit{
		DisableKey: "tenant-x",
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
				GenericKey: &routev3.RateLimit_Action_GenericKey{
					DescriptorValue: "v",
				},
			},
		}},
	})

	// Seed 13 — §5.2 Arm 2: extension descriptor action.
	addSeed(&routev3.RateLimit{
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_Extension{
				Extension: &corev3.TypedExtensionConfig{Name: "ext"},
			},
		}},
	})

	// Seed 14 — §5.2 Arm 3: deprecated dynamic_metadata action.
	addSeed(&routev3.RateLimit{
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_DynamicMetadata{
				DynamicMetadata: &routev3.RateLimit_Action_DynamicMetaData{
					DescriptorKey: "dk",
					MetadataKey:   nil,
				},
			},
		}},
	})

	// -------------------------------------------------------------------------
	// Seeds 15-19: each CORE action — single-policy route-shape proto bytes
	// for the engine surface.
	// -------------------------------------------------------------------------

	// Seed 15 — generic_key action.
	addSeed(&routev3.RateLimit{
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
				GenericKey: &routev3.RateLimit_Action_GenericKey{
					DescriptorKey:   "tenant",
					DescriptorValue: "alpha",
				},
			},
		}},
	})

	// Seed 16 — request_headers action.
	addSeed(&routev3.RateLimit{
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_RequestHeaders_{
				RequestHeaders: &routev3.RateLimit_Action_RequestHeaders{
					HeaderName:    "x-tenant",
					DescriptorKey: "tenant",
					SkipIfAbsent:  false,
				},
			},
		}},
	})

	// Seed 17 — remote_address action.
	addSeed(&routev3.RateLimit{
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_RemoteAddress_{
				RemoteAddress: &routev3.RateLimit_Action_RemoteAddress{},
			},
		}},
	})

	// Seed 18 — destination_cluster action.
	addSeed(&routev3.RateLimit{
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_DestinationCluster_{
				DestinationCluster: &routev3.RateLimit_Action_DestinationCluster{},
			},
		}},
	})

	// Seed 19 — header_value_match action.
	addSeed(&routev3.RateLimit{
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_HeaderValueMatch_{
				HeaderValueMatch: &routev3.RateLimit_Action_HeaderValueMatch{
					DescriptorKey:   "hm",
					DescriptorValue: "v",
					Headers: []*routev3.HeaderMatcher{{
						Name: "x-tag",
						HeaderMatchSpecifier: &routev3.HeaderMatcher_PresentMatch{
							PresentMatch: true,
						},
					}},
				},
			},
		}},
	})

	// -------------------------------------------------------------------------
	// Seed 20: Empty config (proto-zero RateLimit — arm 1 fires).
	// -------------------------------------------------------------------------
	addSeed(&ratelimitfilterv3.RateLimit{})

	// -------------------------------------------------------------------------
	// Seeds 21-30: Boundary / edge-case neighbors.
	// -------------------------------------------------------------------------

	// Seed 21 — stage exactly 10 (boundary passing per §5.1 arm 3 lte:10).
	{
		c := validRateLimitConfig()
		c.Stage = 10
		addSeed(c)
	}

	// Seed 22 — response_headers_to_add exactly 10 entries (boundary passing).
	{
		c := validRateLimitConfig()
		c.ResponseHeadersToAdd = make([]*corev3.HeaderValueOption, 10)
		for i := range c.ResponseHeadersToAdd {
			c.ResponseHeadersToAdd[i] = &corev3.HeaderValueOption{
				Header: &corev3.HeaderValue{Key: "x-rl", Value: "v"},
			}
		}
		addSeed(c)
	}

	// Seed 23 — timeout zero (proto absent ≡ zero ⇒ default 20ms fires).
	{
		c := validRateLimitConfig()
		c.Timeout = durationpb.New(0)
		addSeed(c)
	}

	// Seed 24 — timeout very large (1h — preserved as-is; no upper clamp).
	{
		c := validRateLimitConfig()
		c.Timeout = durationpb.New(3600_000_000_000)
		addSeed(c)
	}

	// Seed 25 — rate_limited_status 200 (< 400 ⇒ clamps to 429).
	{
		c := validRateLimitConfig()
		c.RateLimitedStatus = &typev3.HttpStatus{Code: typev3.StatusCode(200)}
		addSeed(c)
	}

	// Seed 26 — status_on_error 50 (< 100 ⇒ clamps to 500).
	{
		c := validRateLimitConfig()
		c.StatusOnError = &typev3.HttpStatus{Code: typev3.StatusCode(50)}
		addSeed(c)
	}

	// Seed 27 — status_on_error 600 (> 511 ⇒ clamps to 500).
	{
		c := validRateLimitConfig()
		c.StatusOnError = &typev3.HttpStatus{Code: typev3.StatusCode(600)}
		addSeed(c)
	}

	// Seed 28 — enable_x_ratelimit_headers DRAFT_VERSION_03 (parsed-not-emitted
	// at 24.1 per D-RL7; exercises the enum-parse branch).
	{
		c := validRateLimitConfig()
		c.EnableXRatelimitHeaders = ratelimitfilterv3.RateLimit_DRAFT_VERSION_03
		addSeed(c)
	}

	// Seed 29 — stat_prefix populated (AMEND-1 cluster-scoped modulator).
	{
		c := validRateLimitConfig()
		c.StatPrefix = "ingress"
		addSeed(c)
	}

	// Seed 30 — request_type "" (empty ⇒ defaults to "both" per AMEND-3).
	{
		c := validRateLimitConfig()
		c.RequestType = ""
		addSeed(c)
	}

	// -------------------------------------------------------------------------
	// Seed 31: Raw garbage bytes — verifies the proto-Unmarshal-failure path
	// returns (nil, error) cleanly via the wrapped "typed_config unmarshal"
	// branch in buildCompiledConfig.
	// -------------------------------------------------------------------------
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff})

	// -------------------------------------------------------------------------
	// 24.2 Task 7 extension — Seeds 32-46 (corpus only; no new fuzzer).
	// Cross-references parent SPEC §6.9 + PLAN.md D-RL16 corpus delta.
	// -------------------------------------------------------------------------

	// Seed 32 — source_cluster action (24.2 Task 1; AMEND-11 node-cluster
	// threading). Single-policy route-shape proto bytes.
	addSeed(&routev3.RateLimit{
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_SourceCluster_{
				SourceCluster: &routev3.RateLimit_Action_SourceCluster{},
			},
		}},
	})

	// Seed 33 — masked_remote_address action (24.2 Task 1). v4/v6 mask lengths
	// left absent ⇒ proto defaults (32 / 128) fire.
	addSeed(&routev3.RateLimit{
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_MaskedRemoteAddress_{
				MaskedRemoteAddress: &routev3.RateLimit_Action_MaskedRemoteAddress{
					V4PrefixMaskLen: wrapperspb.UInt32(24),
					V6PrefixMaskLen: wrapperspb.UInt32(64),
				},
			},
		}},
	})

	// Seed 34 — metadata action (24.2 Task 1; D-RL8 accessor). DYNAMIC source +
	// single-segment path + default_value populated.
	addSeed(&routev3.RateLimit{
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_Metadata{
				Metadata: &routev3.RateLimit_Action_MetaData{
					DescriptorKey: "tenant",
					MetadataKey: &metadatav3.MetadataKey{
						Key: "envoy.filters.http.jwt_authn",
						Path: []*metadatav3.MetadataKey_PathSegment{{
							Segment: &metadatav3.MetadataKey_PathSegment_Key{Key: "tenant"},
						}},
					},
					DefaultValue: "anon",
					Source:       routev3.RateLimit_Action_MetaData_DYNAMIC,
					SkipIfAbsent: false,
				},
			},
		}},
	})

	// Seed 35 — query_parameters action (24.2 Task 1). Skip-if-absent false ⇒
	// missing query-param drops the descriptor entry (§4.5).
	addSeed(&routev3.RateLimit{
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_QueryParameters_{
				QueryParameters: &routev3.RateLimit_Action_QueryParameters{
					QueryParameterName: "api_key",
					DescriptorKey:      "key",
					SkipIfAbsent:       false,
				},
			},
		}},
	})

	// Seed 36 — query_parameter_value_match action (24.2 Task 1). PresentMatch
	// on a single qp name; descriptor_value populated; expect_match left nil
	// ⇒ engine defaults to true.
	addSeed(&routev3.RateLimit{
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_QueryParameterValueMatch_{
				QueryParameterValueMatch: &routev3.RateLimit_Action_QueryParameterValueMatch{
					DescriptorKey:   "qpm",
					DescriptorValue: "v",
					QueryParameters: []*routev3.QueryParameterMatcher{{
						Name: "tag",
						QueryParameterMatchSpecifier: &routev3.QueryParameterMatcher_PresentMatch{
							PresentMatch: true,
						},
					}},
				},
			},
		}},
	})

	// Seed 37 — RateLimitPerRoute with vh_rate_limits=OVERRIDE (proto-zero
	// default; AMEND-5). Exercises Surface 3 happy-path validator + projection.
	addSeed(&ratelimitfilterv3.RateLimitPerRoute{
		VhRateLimits: ratelimitfilterv3.RateLimitPerRoute_OVERRIDE,
	})

	// Seed 38 — RateLimitPerRoute with vh_rate_limits=INCLUDE (Axis-B
	// cross-tier-include arm; Task 4 consumer).
	addSeed(&ratelimitfilterv3.RateLimitPerRoute{
		VhRateLimits: ratelimitfilterv3.RateLimitPerRoute_INCLUDE,
	})

	// Seed 39 — RateLimitPerRoute with vh_rate_limits=IGNORE (Axis-B
	// vhost-suppression arm; Task 4 consumer).
	addSeed(&ratelimitfilterv3.RateLimitPerRoute{
		VhRateLimits: ratelimitfilterv3.RateLimitPerRoute_IGNORE,
	})

	// Seed 40 — RateLimitPerRoute with override_option=OVERRIDE_POLICY
	// (AMEND-4 PARSE-ACCEPTED-but-IGNORED arm; validator must not error).
	addSeed(&ratelimitfilterv3.RateLimitPerRoute{
		OverrideOption: ratelimitfilterv3.RateLimitPerRoute_OVERRIDE_POLICY,
	})

	// Seed 41 — RateLimitPerRoute with override_option=INCLUDE_POLICY
	// (AMEND-4 PARSE-ACCEPTED-but-IGNORED arm).
	addSeed(&ratelimitfilterv3.RateLimitPerRoute{
		OverrideOption: ratelimitfilterv3.RateLimitPerRoute_INCLUDE_POLICY,
	})

	// Seed 42 — RateLimitPerRoute with override_option=IGNORE_POLICY +
	// non-empty domain (AMEND-4 PARSE-ACCEPTED-but-IGNORED arm + per-route
	// domain override). Domain wins-discipline at Task 4.
	addSeed(&ratelimitfilterv3.RateLimitPerRoute{
		OverrideOption: ratelimitfilterv3.RateLimitPerRoute_IGNORE_POLICY,
		Domain:         "tenant-override",
	})

	// Seed 43 — per-policy stage=5 (24.2 Task 2 multi-stage bucketing; the
	// new arm under §4.4). Surface 2 ValidateRouteRateLimits must accept;
	// engine `bucketRateLimitsByStage` slot 5 holds the policy.
	addSeed(&routev3.RateLimit{
		Stage: wrapperspb.UInt32(5),
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
				GenericKey: &routev3.RateLimit_Action_GenericKey{DescriptorValue: "s5"},
			},
		}},
	})

	// Seed 44 — per-policy stage=11 (24.2 Task 2 PARSE-REJECT arm; the new
	// per-policy stage > 10 arm under §4.4 + §5.1 Arm 3 mirror). Surface 2
	// ValidateRouteRateLimits must reject byte-stable.
	addSeed(&routev3.RateLimit{
		Stage: wrapperspb.UInt32(11),
		Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
				GenericKey: &routev3.RateLimit_Action_GenericKey{DescriptorValue: "s11"},
			},
		}},
	})

	// Seed 45 — X-RateLimit DRAFT_VERSION_03 + stat_prefix populated +
	// disable_x_envoy_ratelimited_header=true (24.2 Task 5 toggle arm). The
	// 24.1 Seed 28 covers the toggle in isolation; this seed pairs it with
	// the cluster-scoped modulator + the legacy x-envoy-ratelimited
	// disable-flag to exercise the full headers-side combination.
	{
		c := validRateLimitConfig()
		c.EnableXRatelimitHeaders = ratelimitfilterv3.RateLimit_DRAFT_VERSION_03
		c.StatPrefix = "ingress"
		c.DisableXEnvoyRatelimitedHeader = true
		addSeed(c)
	}

	// Seed 46 — legacy `RouteAction.include_vh_rate_limits=true` proto bytes
	// (AMEND-5 legacy force-include arm; D-RL10). The fuzz body cannot
	// type-assert through a `routev3.RouteAction` (the legacy bool is
	// threaded via the DCB at request time, not through a TypeURL envelope),
	// but the byte shape still exercises the proto-Unmarshal-mismatch
	// defensive paths in all 3 surfaces — must-never-panic on a coherently-
	// shaped proto whose wire-tags happen to overlap with the 3 target
	// shapes' fields (e.g., the bool's varint may collide with another
	// field's varint at the same tag number).
	addSeed(&routev3.RouteAction{
		ClusterSpecifier:    &routev3.RouteAction_Cluster{Cluster: "upstream_xyz"},
		IncludeVhRateLimits: wrapperspb.Bool(true),
	})

	// -------------------------------------------------------------------------
	// Fixed engine inputs for the §4 surface coverage. The engine is pure;
	// these fixed inputs exercise the 5 CORE actions on every fuzz iteration
	// while the variable input is the route-shape unmarshal attempt below.
	// -------------------------------------------------------------------------
	fixedRouteRLs := []*routev3.RateLimit{
		{Actions: []*routev3.RateLimit_Action{{
			ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
				GenericKey: &routev3.RateLimit_Action_GenericKey{DescriptorValue: "v"},
			},
		}}},
	}
	fixedHeaders := http.Header{"X-Tenant": []string{"alpha"}}
	fixedRemoteAddr := net.Addr(&net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 1234})
	const fixedClusterName = "upstream_xyz"

	// -------------------------------------------------------------------------
	// Fuzz body — must-never-panic structural assertion across BOTH surfaces.
	// -------------------------------------------------------------------------
	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("ratelimit fuzz panicked: %v\nInput: %x", r, data)
			}
		}()

		// Surface 1: buildCompiledConfig over the typed_config Any envelope.
		// Empty FactoryCtx{} ⇒ §5.1 arm 10 PARSE-REJECT for any input that
		// makes it past arms 1-9; earlier-arm inputs surface their own
		// byte-stable error. EITHER outcome is no-panic.
		typedConfig := &anypb.Any{TypeUrl: TypeURL, Value: data}
		_, _ = buildCompiledConfig(typedConfig, envoyhttp.FactoryCtx{})

		// Surface 2: ValidateRouteRateLimits + buildDescriptors over a route-
		// shape interpretation of the same bytes. Many random inputs will
		// fail to unmarshal as routev3.RateLimit — that's fine; the no-panic
		// invariant is the only contract.
		var rl routev3.RateLimit
		if err := proto.Unmarshal(data, &rl); err == nil {
			rls := []*routev3.RateLimit{&rl}
			_ = ValidateRouteRateLimits(rls)
			// Engine call — drives the FULL 10-action dispatch (24.2 Task 1
			// extended the §4 action roster from the 24.1 CORE-5 to the
			// full 10) + the vacuous-true matcher AND-fold + the §4.5
			// drop/skip discipline. The fixed inputs exercise the
			// generic_key happy path; the variable rl exercises arbitrary
			// action shapes.
			_ = buildDescriptors(rls, nil, fixedHeaders, fixedRemoteAddr, fixedClusterName)
			_ = buildDescriptors(fixedRouteRLs, rls, fixedHeaders, fixedRemoteAddr, fixedClusterName)
		}

		// Surface 3: validatePerRouteRateLimit + compilePerRouteForRequest
		// over a per-route-shape interpretation of the same bytes (24.2
		// Task 3 — the 10th canonical TPFC compile per ADR-0199). Many
		// random inputs will fail to unmarshal as
		// `ratelimitfilterv3.RateLimitPerRoute` — that's fine; the no-panic
		// invariant is the only contract. When unmarshal succeeds, drive
		// the ADR-0110 single-chokepoint validator AND the request-time
		// projection — both must tolerate arbitrary input shapes (including
		// embedded `rate_limits[]` slices with §5.2 PARSE-REJECT arms +
		// per-policy stage > 10 + arbitrary `vh_rate_limits` /
		// `override_option` enum varints).
		var pr ratelimitfilterv3.RateLimitPerRoute
		if err := proto.Unmarshal(data, &pr); err == nil {
			_ = validatePerRouteRateLimit(&pr)
			// Projection is independent of validator — must-never-panic
			// even when the validator would reject (defensive contract per
			// ADR-0085 nil-tolerance + ADR-0018 never-panic).
			_ = compilePerRouteForRequest(&pr)
		}
	})
}
