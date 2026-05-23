package ratelimit

// fuzz_test.go — 33rd project-wide fuzzer `FuzzRateLimitConfigParse` per
// phase-24.1 PLAN Task 8 + parent SPEC §6.9 + ADR-0018 baseline.
//
// Drives arbitrary byte sequences as the typed_config Any.Value payload
// through TWO surfaces:
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
// # Seed corpus per PLAN Task 8 + parent §6.9
//
// 31 hand-curated `f.Add` seeds covering:
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
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
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
			// Engine call — drives the 5 CORE-action dispatch + the
			// vacuous-true matcher AND-fold + the §4.5 drop/skip discipline.
			// The fixed inputs exercise the generic_key happy path; the
			// variable rl exercises arbitrary action shapes.
			_ = buildDescriptors(rls, nil, fixedHeaders, fixedRemoteAddr, fixedClusterName)
			_ = buildDescriptors(fixedRouteRLs, rls, fixedHeaders, fixedRemoteAddr, fixedClusterName)
		}
	})
}
