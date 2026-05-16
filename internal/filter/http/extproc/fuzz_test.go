package extproc

import (
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// FuzzExtProcConfigParse fuzzes arbitrary byte sequences as the typed_config
// Any.Value payload to New. Asserts the structural contract per ADR-0018 +
// ADR-0167 + ADR-0168: New returns either (factory, nil) OR (nil, error);
// never panics; never returns (nil, nil); never blocks. 24th fuzzer overall
// (phases 02-18.2 contributed 23; phase 19.1 adds this one per SPEC §7.3).
//
// Seed corpus per SPEC §7.3 — 8 valid + parse-reject variants exercising the
// dual-mode envelope, the body/trailer/STREAMED PARSE-REJECT branches, the
// error-posture surface, the per-route 5th-canonical, the mutation_rules /
// forward_rules / route_cache_action interactions, and the GoogleGrpc /
// both-set / neither-set PARSE-REJECTs:
//
//	(0)  grpc_service envoy_grpc valid cluster_name — PARSE-REJECT under
//	     empty FactoryCtx (cluster manager nil); structurally valid.
//	(1)  http_service with http_uri populated — valid; headers-only body
//	     mode is auto-resolved.
//	(2)  both grpc_service AND http_service set — PARSE-REJECT (parent
//	     §5.P1 mutual-exclusion).
//	(3)  neither grpc_service NOR http_service set — PARSE-REJECT
//	     (parent §5.P1 mutual-exclusion).
//	(4)  observability_mode=true — PARSE-REJECT (STREAMED-only flag;
//	     parent §5.P10).
//	(5)  send_body_without_waiting_for_header_response=true —
//	     PARSE-REJECT (STREAMED-only flag).
//	(6)  body-mode != NONE — PARSE-REJECT in 19.1 (body activates at
//	     19.2 per ADR-0168 AMENDMENT path).
//	(7)  trailer-mode != SKIP — PARSE-REJECT permanently (parent §5.P9).
//	(8)  failure_mode_allow + message_timeout + max_message_timeout +
//	     disable_immediate_response — full error-posture surface.
//	(9)  GoogleGrpc arm — PARSE-REJECT envoy-go-strict per ADR-0157
//	     §Decision AMENDMENT (inherited by ext_proc).
//	(10) allow_mode_override + allowed_override_modes populated —
//	     valid override allowlist.
//	(11) route_cache_action AND disable_clear_route_cache both set —
//	     PARSE-REJECT (parent §5.P5 mutual-exclusion).
//	(12) per-route ExtProcPerRoute disabled:true (raw bytes used as
//	     ExternalProcessor — exercises the unmarshal-into-wrong-type
//	     PARSE-REJECT path).
//	(13) per-route ExtProcPerRoute overrides{processing_mode} —
//	     same unmarshal-into-wrong-type PARSE-REJECT exercise.
//
// The fuzz function body is intentionally minimal — only the structural
// contract (never-both-nil; never-both-set; never-panic) is asserted. The
// fuzz engine derives further inputs from these seeds at the 30s budget per
// ADR-0018 short-mode CI policy.
func FuzzExtProcConfigParse(f *testing.F) {
	addSeed := func(msg proto.Message) {
		f.Helper()
		raw, err := proto.Marshal(msg)
		if err != nil {
			f.Fatalf("seed marshal: %v", err)
		}
		f.Add(raw)
	}

	// Helper: minimal grpc_service envoy_grpc with cluster_name.
	grpcEnvoy := func(cluster string) *corev3.GrpcService {
		return &corev3.GrpcService{
			TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{
					ClusterName: cluster,
				},
			},
			Timeout: &durationpb.Duration{Seconds: 1},
		}
	}

	// (0) grpc_service envoy_grpc valid cluster_name — structurally valid.
	addSeed(&extprocv3.ExternalProcessor{
		GrpcService:    grpcEnvoy("c_ext_proc"),
		MessageTimeout: &durationpb.Duration{Seconds: 1},
	})

	// (1) http_service with http_uri populated — valid headers-only.
	addSeed(&extprocv3.ExternalProcessor{
		HttpService: &extprocv3.ExtProcHttpService{
			HttpService: &corev3.HttpService{
				HttpUri: &corev3.HttpUri{
					Uri:     "http://127.0.0.1:8765/process",
					Timeout: &durationpb.Duration{Seconds: 1},
				},
			},
		},
	})

	// (2) Both grpc_service AND http_service set — PARSE-REJECT (mutex).
	addSeed(&extprocv3.ExternalProcessor{
		GrpcService: grpcEnvoy("c_ext_proc"),
		HttpService: &extprocv3.ExtProcHttpService{
			HttpService: &corev3.HttpService{
				HttpUri: &corev3.HttpUri{Uri: "http://127.0.0.1:8765/process"},
			},
		},
	})

	// (3) Neither set — PARSE-REJECT (mutex).
	addSeed(&extprocv3.ExternalProcessor{
		MessageTimeout: &durationpb.Duration{Seconds: 1},
	})

	// (4) observability_mode=true — PARSE-REJECT (STREAMED-only flag).
	addSeed(&extprocv3.ExternalProcessor{
		GrpcService:       grpcEnvoy("c_ext_proc"),
		ObservabilityMode: true,
	})

	// (5) send_body_without_waiting_for_header_response=true — PARSE-REJECT.
	addSeed(&extprocv3.ExternalProcessor{
		GrpcService:                             grpcEnvoy("c_ext_proc"),
		SendBodyWithoutWaitingForHeaderResponse: true,
	})

	// (6) body-mode != NONE — PARSE-REJECT in 19.1.
	addSeed(&extprocv3.ExternalProcessor{
		GrpcService: grpcEnvoy("c_ext_proc"),
		ProcessingMode: &extprocv3.ProcessingMode{
			RequestBodyMode: extprocv3.ProcessingMode_BUFFERED,
		},
	})

	// (7) trailer-mode != SKIP — PARSE-REJECT permanently.
	addSeed(&extprocv3.ExternalProcessor{
		GrpcService: grpcEnvoy("c_ext_proc"),
		ProcessingMode: &extprocv3.ProcessingMode{
			RequestTrailerMode: extprocv3.ProcessingMode_SEND,
		},
	})

	// (8) Full error-posture surface — failure_mode_allow + message_timeout +
	// max_message_timeout + disable_immediate_response.
	addSeed(&extprocv3.ExternalProcessor{
		GrpcService:              grpcEnvoy("c_ext_proc"),
		FailureModeAllow:         true,
		MessageTimeout:           &durationpb.Duration{Nanos: 500 * 1000 * 1000}, // 500ms
		MaxMessageTimeout:        &durationpb.Duration{Seconds: 5},
		DisableImmediateResponse: true,
	})

	// (9) GoogleGrpc arm — PARSE-REJECT envoy-go-strict.
	addSeed(&extprocv3.ExternalProcessor{
		GrpcService: &corev3.GrpcService{
			TargetSpecifier: &corev3.GrpcService_GoogleGrpc_{
				GoogleGrpc: &corev3.GrpcService_GoogleGrpc{
					TargetUri:  "extproc.example.com:443",
					StatPrefix: "ext_proc_grpc",
				},
			},
		},
	})

	// (10) allow_mode_override + allowed_override_modes populated.
	addSeed(&extprocv3.ExternalProcessor{
		GrpcService:       grpcEnvoy("c_ext_proc"),
		AllowModeOverride: true,
		ProcessingMode: &extprocv3.ProcessingMode{
			RequestHeaderMode:  extprocv3.ProcessingMode_SEND,
			ResponseHeaderMode: extprocv3.ProcessingMode_SEND,
		},
		AllowedOverrideModes: []*extprocv3.ProcessingMode{
			{
				RequestHeaderMode:  extprocv3.ProcessingMode_SKIP,
				ResponseHeaderMode: extprocv3.ProcessingMode_SEND,
			},
			{
				RequestHeaderMode:  extprocv3.ProcessingMode_SEND,
				ResponseHeaderMode: extprocv3.ProcessingMode_SKIP,
			},
		},
	})

	// (11) route_cache_action AND disable_clear_route_cache both set —
	// PARSE-REJECT (parent §5.P5 mutex).
	addSeed(&extprocv3.ExternalProcessor{
		GrpcService:            grpcEnvoy("c_ext_proc"),
		RouteCacheAction:       extprocv3.ExternalProcessor_CLEAR,
		DisableClearRouteCache: true,
	})

	// (12) per-route ExtProcPerRoute disabled:true — raw bytes used as
	// ExternalProcessor exercises the unmarshal-into-wrong-type PARSE-REJECT
	// path. The fuzzer asserts the structural contract regardless of where
	// in the parse pipeline the rejection fires.
	{
		pr := &extprocv3.ExtProcPerRoute{
			Override: &extprocv3.ExtProcPerRoute_Disabled{Disabled: true},
		}
		raw, _ := proto.Marshal(pr)
		f.Add(raw)
	}

	// (13) per-route ExtProcPerRoute overrides{processing_mode} — same
	// wrong-type exercise.
	{
		pr := &extprocv3.ExtProcPerRoute{
			Override: &extprocv3.ExtProcPerRoute_Overrides{
				Overrides: &extprocv3.ExtProcOverrides{
					ProcessingMode: &extprocv3.ProcessingMode{
						RequestHeaderMode: extprocv3.ProcessingMode_SKIP,
					},
				},
			},
		}
		raw, _ := proto.Marshal(pr)
		f.Add(raw)
	}

	// Empty bytes — Unmarshal succeeds to zero-value ExternalProcessor; the
	// neither-set PARSE-REJECT fires. Contract holds.
	f.Add([]byte{})

	// -------------------------------------------------------------------------
	// Fuzz body: structural contract assertions only.
	// -------------------------------------------------------------------------
	f.Fuzz(func(t *testing.T, raw []byte) {
		// Empty FactoryCtx (no Stats registry, no ClusterManager) per the
		// extauthz fuzzer precedent: this fuzzer targets the typed_config
		// Any-unmarshal + parse pipeline, not the stats-registration / dial
		// paths. buildCompiledConfig short-circuits the stats path on
		// ctx.Stats==nil per ADR-0085 nil-tolerance; the gRPC dial path
		// PARSE-REJECTs under nil ClusterManager — a coherent contract-
		// preserving outcome.
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
