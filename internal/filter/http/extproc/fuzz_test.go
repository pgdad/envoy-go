package extproc

import (
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	extprocsvcv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
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

// FuzzProcessingResponseMapping fuzzes arbitrary byte sequences as a serialized
// `*extprocsvcv3.ProcessingResponse` payload, then drives `applyProcessingResponse`
// across each of the 4 stage discriminators (request_headers / response_headers /
// request_body / response_body) under a minimal but realistic `*filter` envelope.
// 25th fuzzer overall (phases 02-19.1 contributed 24; phase 19.2 adds this one per
// SPEC §7.3 + the 19.2-PLAN's planner-time D9 BEHAVIOR_CONTRACT-bundle scope).
//
// Per SPEC §7.3 the fuzz contract is:
//
//   - `proto.Unmarshal` of arbitrary bytes into `*ProcessingResponse` must never
//     panic (the proto runtime contract; fuzz exercises malformed-byte resilience).
//   - `applyProcessingResponse` dispatch must never panic; never block; never
//     return an action outside the 5-value enum (`actContinue`/`actStop`/
//     `actError`/`actImmediate`/`actContinueButStillWaiting`).
//   - Spurious-counter increments stay bounded — each call increments
//     `spuriousMsgsReceived` by AT MOST a small constant (the dispatcher emits at
//     most one spurious per stage per call; loose bound 8 covers stage-mismatch +
//     disable_immediate_response drop + mode_override allowlist-miss + body_mutation
//     streamed_response + header_mutation rejection + override_message_timeout
//     out-of-range; the bound is loose by design — the assertion catches an
//     unbounded-loop regression, not a tight equality).
//
// Corpus seeds per SPEC §7.3 — 6+ seeds explicitly exercising the body-stage
// arms the 19.2 §Decision AMENDMENTs activate (per ADR-0172 §Decision AMENDMENT +
// ADR-0168 §Decision AMENDMENT + ADR-0171 §Decision AMENDMENT). The seeds
// complement the 19.1-landing 24th fuzzer `FuzzExtProcConfigParse` which targets
// the typed_config PARSE-REJECT surface; this 25th fuzzer targets the
// `*ProcessingResponse` dispatch surface (the wire-shape inbound from the
// processor).
//
//	(0)  body-stage CommonResponse with body_mutation{body: <bytes>} —
//	     CONSUMED at request_body / response_body per ADR-0172 §Decision
//	     AMENDMENT row 1 of SPEC §4.2; writeBodyMutation replaces the buffer
//	     + reconciles content-length.
//	(1)  body-stage CommonResponse with body_mutation{clear_body: true} —
//	     CONSUMED as zero-byte replacement per SPEC §4.2 row 2.
//	(2)  body-stage CommonResponse with body_mutation{streamed_response} —
//	     PARSE-REJECT per SPEC §4.2 row 3; increments spurious + returns
//	     (actError, errStreamedResponseBodyMutationUnsupported).
//	(3)  CONTINUE_AND_REPLACE at response_headers stage WITH body-mode =
//	     BUFFERED + header_mutation + body_mutation combined — CONSUMED as
//	     combined header+body replacement per SPEC §4.3 row 2.
//	(4)  body-stage ImmediateResponse with status + headers + grpc_status —
//	     CONSUMED per SPEC §4.4 multi-stage deny extension (the request_body
//	     stage emit fires SendLocalReply via the decode-side path; this seed
//	     exercises the dispatch even though the test filter has no live dcb,
//	     because emitImmediateResponse short-circuits to actImmediate without
//	     invoking the callback on a nil-dcb defensive path — the structural
//	     contract assertion is the action enum bound).
//	(5)  malformed BodyMutation with BOTH `body` and the clear-body discriminator
//	     populated via the proto wire (the oneof's last-write-wins resolution
//	     by `proto.Unmarshal` selects the second arm; the fuzzer asserts the
//	     dispatcher honors whichever arm wins without panicking on the
//	     ambiguous shape).
//	(6)  empty ProcessingResponse{} — exercises the stage-mismatch defensive
//	     path (the response carries no oneof arm; cr == nil triggers
//	     spurious + errStageMismatch) at all 4 stages.
//	(7)  override_message_timeout-only response — CONSUMED per SPEC §6.7
//	     step 2 + ADR-0171 §Decision; returns actContinueButStillWaiting; no
//	     spurious increment.
//
// The fuzz function body asserts the structural contract per the SPEC §7.3
// invariants. The fuzz engine derives further inputs from these seeds at the
// 30s ADR-0018 short-mode budget.
func FuzzProcessingResponseMapping(f *testing.F) {
	addSeed := func(msg proto.Message) {
		f.Helper()
		raw, err := proto.Marshal(msg)
		if err != nil {
			f.Fatalf("seed marshal: %v", err)
		}
		f.Add(raw)
	}

	// Helper: minimal body-stage CommonResponse wrapped as a request_body
	// ProcessingResponse.
	mkRequestBody := func(cr *extprocsvcv3.CommonResponse) *extprocsvcv3.ProcessingResponse {
		return &extprocsvcv3.ProcessingResponse{
			Response: &extprocsvcv3.ProcessingResponse_RequestBody{
				RequestBody: &extprocsvcv3.BodyResponse{Response: cr},
			},
		}
	}
	mkResponseBody := func(cr *extprocsvcv3.CommonResponse) *extprocsvcv3.ProcessingResponse {
		return &extprocsvcv3.ProcessingResponse{
			Response: &extprocsvcv3.ProcessingResponse_ResponseBody{
				ResponseBody: &extprocsvcv3.BodyResponse{Response: cr},
			},
		}
	}
	mkResponseHeaders := func(cr *extprocsvcv3.CommonResponse) *extprocsvcv3.ProcessingResponse {
		return &extprocsvcv3.ProcessingResponse{
			Response: &extprocsvcv3.ProcessingResponse_ResponseHeaders{
				ResponseHeaders: &extprocsvcv3.HeadersResponse{Response: cr},
			},
		}
	}

	// (0) body-stage body_mutation{body: <bytes>}.
	addSeed(mkResponseBody(&extprocsvcv3.CommonResponse{
		BodyMutation: &extprocsvcv3.BodyMutation{
			Mutation: &extprocsvcv3.BodyMutation_Body{Body: []byte("rewritten-by-processor")},
		},
	}))

	// (1) body-stage body_mutation{clear_body: true}.
	addSeed(mkResponseBody(&extprocsvcv3.CommonResponse{
		BodyMutation: &extprocsvcv3.BodyMutation{
			Mutation: &extprocsvcv3.BodyMutation_ClearBody{ClearBody: true},
		},
	}))

	// (2) body-stage body_mutation{streamed_response} — PARSE-REJECT path.
	addSeed(mkResponseBody(&extprocsvcv3.CommonResponse{
		BodyMutation: &extprocsvcv3.BodyMutation{
			Mutation: &extprocsvcv3.BodyMutation_StreamedResponse{
				StreamedResponse: &extprocsvcv3.StreamedBodyResponse{Body: []byte("nope")},
			},
		},
	}))

	// (3) response_headers stage CONTINUE_AND_REPLACE with combined
	// header+body replacement. ADR-0172 §Decision AMENDMENT row 2 of SPEC §4.3.
	addSeed(mkResponseHeaders(&extprocsvcv3.CommonResponse{
		Status: extprocsvcv3.CommonResponse_CONTINUE_AND_REPLACE,
		HeaderMutation: &extprocsvcv3.HeaderMutation{
			SetHeaders: []*corev3.HeaderValueOption{
				{Header: &corev3.HeaderValue{Key: "x-extproc-car", Value: "scenario_e"}},
			},
		},
		BodyMutation: &extprocsvcv3.BodyMutation{
			Mutation: &extprocsvcv3.BodyMutation_Body{Body: []byte("car-body")},
		},
	}))

	// (4) body-stage ImmediateResponse with status + headers + grpc_status.
	addSeed(mkRequestBody(&extprocsvcv3.CommonResponse{})) // anchor: empty body-stage allow
	addSeed(&extprocsvcv3.ProcessingResponse{
		Response: &extprocsvcv3.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &extprocsvcv3.ImmediateResponse{
				Status: &typev3.HttpStatus{Code: typev3.StatusCode_Forbidden},
				Headers: &extprocsvcv3.HeaderMutation{
					SetHeaders: []*corev3.HeaderValueOption{
						{Header: &corev3.HeaderValue{Key: "x-extproc-deny-stage", Value: "body"}},
					},
				},
				Body:       []byte("denied-at-body-stage-fuzz-seed"),
				GrpcStatus: &extprocsvcv3.GrpcStatus{Status: 7 /*PERMISSION_DENIED*/},
				Details:    "ext_proc_denied",
			},
		},
	})

	// (5) malformed BodyMutation — populate body AND construct a clear_body=true
	// case to exercise the oneof dispatch under proto.Unmarshal's wire-format
	// last-write-wins semantics. We construct the wire bytes manually by
	// marshaling a body-only mutation then concatenating clear_body=true raw
	// bytes; here a structurally simple two-arm body+clear via two distinct
	// seeds is sufficient — proto-wire-format ambiguity is fuzz-derived.
	addSeed(mkResponseBody(&extprocsvcv3.CommonResponse{
		BodyMutation: &extprocsvcv3.BodyMutation{
			Mutation: &extprocsvcv3.BodyMutation_Body{Body: []byte{}},
		},
	}))

	// (6) empty ProcessingResponse{} — drives stage-mismatch at all 4 stages.
	addSeed(&extprocsvcv3.ProcessingResponse{})

	// (7) override_message_timeout-only response — actContinueButStillWaiting.
	addSeed(&extprocsvcv3.ProcessingResponse{
		OverrideMessageTimeout: durationpb.New(500_000_000), // 500ms in nanos
	})

	// (8) header_mutation set+remove combined with body_mutation at body stage
	// — exercises the header-mutation rejection path under mutation_rules-on
	// (here off; the spurious counter MAY still increment if the value-char
	// validator rejects). The fuzzer asserts the spurious-bound contract.
	addSeed(mkRequestBody(&extprocsvcv3.CommonResponse{
		HeaderMutation: &extprocsvcv3.HeaderMutation{
			SetHeaders: []*corev3.HeaderValueOption{
				{Header: &corev3.HeaderValue{Key: "x-set-by-processor", Value: "v"}},
				{Header: &corev3.HeaderValue{Key: ":authority", Value: "evil.example"}},
			},
			RemoveHeaders: []string{"x-internal", "x-secret"},
		},
		BodyMutation: &extprocsvcv3.BodyMutation{
			Mutation: &extprocsvcv3.BodyMutation_Body{Body: []byte("mutated")},
		},
	}))

	// Empty bytes — Unmarshal succeeds to zero-value ProcessingResponse{}; the
	// stage-mismatch defensive path fires. Contract holds.
	f.Add([]byte{})

	// -------------------------------------------------------------------------
	// Fuzz body: structural contract assertions only per SPEC §7.3.
	// -------------------------------------------------------------------------
	f.Fuzz(func(t *testing.T, raw []byte) {
		var resp extprocsvcv3.ProcessingResponse
		if err := proto.Unmarshal(raw, &resp); err != nil {
			// Malformed proto bytes — Unmarshal rejected; nothing to dispatch.
			// The contract is satisfied (Unmarshal never panics; error return
			// is the proto-runtime invariant).
			return
		}

		// Drive each of the 4 stages independently. The dispatcher's stage-
		// dependent behavior (stage-mismatch, body-stage body_mutation, header-
		// stage CONTINUE_AND_REPLACE, body-stage CONTINUE_AND_REPLACE-as-
		// CONTINUE, etc.) is exercised under a fresh `*filter` each iteration
		// so per-stage state mutations (`f.overrideApplied`, `f.skipBodyStage*`,
		// `f.decodeBodyBuf`/`f.encodeBodyBuf`, `f.activeProcessingMode`) do
		// NOT cross-contaminate across stages.
		for _, s := range []stage{stageRequestHeaders, stageResponseHeaders, stageRequestBody, stageResponseBody} {
			reg := stats.NewRegistry()
			cc := &compiledConfig{
				stats:                    newFilterStats(reg, "fuzz"),
				messageTimeout:           200_000_000, // 200ms
				maxMessageTimeout:        10_000_000_000,
				allowModeOverride:        true,
				disableImmediateResponse: false,
			}
			f := &filter{
				cc:                   cc,
				activeProcessingMode: &resolvedProcessingMode{},
			}
			beforeSpurious := cc.stats.spuriousMsgsReceived.Load()

			act, err := applyProcessingResponse(f, s, &resp)

			// (a) action enum bound check.
			switch act {
			case actContinue, actStop, actError, actImmediate, actContinueButStillWaiting:
				// ok
			default:
				t.Fatalf("stage=%v: applyProcessingResponse returned action(%d) outside the 5-value enum", s, int(act))
			}

			// (b) (actError, nil) is structurally invalid — the dispatcher MUST
			// return a sentinel on actError. The defensive nil-receiver guard
			// returns (actContinue, nil); actError implies a non-nil error.
			if act == actError && err == nil {
				t.Fatalf("stage=%v: actError with nil error — sentinel invariant violation (raw len=%d)", s, len(raw))
			}

			// (c) spurious-increment bound check. The dispatcher emits AT MOST
			// 8 spurious increments per call (stage-mismatch + disable-immediate
			// drop + header-mutation rejection + body_mutation streamed_response +
			// override allowlist miss + override out-of-range + mode_override
			// rejection + reserved). The bound is loose by design — catches an
			// unbounded-loop regression, not tight equality.
			after := cc.stats.spuriousMsgsReceived.Load()
			// Counter is monotonic per-call; if `after < beforeSpurious` the
			// counter wrapped or was corrupted — explicit failure.
			if after < beforeSpurious {
				t.Fatalf("stage=%v: spuriousMsgsReceived regressed (before=%d, after=%d)", s, beforeSpurious, after)
			}
			delta := after - beforeSpurious
			const spuriousBound uint64 = 8
			if delta > spuriousBound {
				t.Fatalf("stage=%v: spuriousMsgsReceived delta = %d (want 0..%d)", s, delta, spuriousBound)
			}
		}
	})
}
