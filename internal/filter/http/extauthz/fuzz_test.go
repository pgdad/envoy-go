package extauthz

import (
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ext_authzv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	rpcstatus "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// FuzzExtAuthzConfigParse fuzzes arbitrary byte sequences as the typed_config
// Any.Value payload to New. Asserts the contract per ADR-0018 + ADR-0156: New
// returns either (factory, nil) OR (nil, error); never panics; never returns
// (nil, nil). 22nd fuzzer overall (phases 02–17 contributed 21; phase 18.1
// contributed this fuzzer at its initial 21-seed corpus; phase 18.2 Task 9
// extends the corpus with 8 grpc_service variants per SPEC §7.3).
//
// Seed corpus per SPEC §7.3 — covering each-decision + boundary cases:
//
//	(0)  http_service valid (minimal server_uri)
//	(1)  http_service empty uri (PARSE-REJECT path)
//	(2)  grpc_service (PARSE-REJECT path)
//	(3)  empty services oneof (PARSE-REJECT path)
//	(4)  with_request_body max_request_bytes 0 (PARSE-REJECT)
//	(5)  with_request_body max_request_bytes positive (valid)
//	(6)  per-route disabled: true (valid)
//	(7)  per-route disabled: false (PARSE-REJECT per PGV const:true)
//	(8)  per-route check_settings (valid)
//	(9)  allowed_headers exact matcher
//	(10) allowed_headers prefix matcher
//	(11) allowed_headers suffix matcher
//	(12) allowed_headers contains matcher
//	(13) allowed_headers safe_regex matcher (RE2-compatible)
//	(14) allowed_headers custom matcher (PARSE-REJECT)
//	(15) disallowed_headers exact matcher
//	(16) error-posture: failure_mode_allow + failure_mode_allow_header_add + status_on_error
//	(17) full http_service with authorization_request + authorization_response
//	(18) non-V3 transport_api_version (PARSE-REJECT)
//	(19) with_request_body allow_partial_message + pack_as_bytes
//	(20) validate_mutations true
//
// The fuzz function body is intentionally minimal — only the structural
// contract (never-both-nil; never-both-set; never-panic) is asserted. The fuzz
// engine derives further inputs from these seeds at the 30s budget per
// ADR-0018 short-mode CI policy.
func FuzzExtAuthzConfigParse(f *testing.F) {
	// addSeed marshals msg + adds the resulting raw bytes to the fuzzer corpus.
	// Mirrors the phase-16/17 fuzzer helper pattern.
	addSeed := func(msg proto.Message) {
		f.Helper()
		raw, err := proto.Marshal(msg)
		if err != nil {
			f.Fatalf("seed marshal: %v", err)
		}
		f.Add(raw)
	}

	// Minimal valid HttpService helper.
	minHTTPService := func(uri string) *ext_authzv3.ExtAuthz_HttpService {
		return &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{
					Uri:     uri,
					Timeout: &durationpb.Duration{Seconds: 1},
				},
			},
		}
	}

	// (0) http_service valid — minimal server_uri.
	addSeed(&ext_authzv3.ExtAuthz{
		Services:            minHTTPService("http://127.0.0.1:9191/auth"),
		TransportApiVersion: corev3.ApiVersion_V3,
	})

	// (1) http_service with empty uri — PARSE-REJECT path.
	addSeed(&ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{Uri: ""},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
	})

	// (2) grpc_service — PARSE-REJECT in 18.1 (grpc_service mode not yet supported).
	addSeed(&ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_GrpcService{
			GrpcService: &corev3.GrpcService{},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
	})

	// (3) Empty services oneof — PARSE-REJECT.
	addSeed(&ext_authzv3.ExtAuthz{
		TransportApiVersion: corev3.ApiVersion_V3,
	})

	// (4) with_request_body max_request_bytes == 0 — PARSE-REJECT (PGV-mirror).
	addSeed(&ext_authzv3.ExtAuthz{
		Services:            minHTTPService("http://127.0.0.1:9191/auth"),
		TransportApiVersion: corev3.ApiVersion_V3,
		WithRequestBody: &ext_authzv3.BufferSettings{
			MaxRequestBytes: 0,
		},
	})

	// (5) with_request_body max_request_bytes positive — valid.
	addSeed(&ext_authzv3.ExtAuthz{
		Services:            minHTTPService("http://127.0.0.1:9191/auth"),
		TransportApiVersion: corev3.ApiVersion_V3,
		WithRequestBody: &ext_authzv3.BufferSettings{
			MaxRequestBytes: 8192,
		},
	})

	// (6) per-route disabled: true — valid (5th-canonical disabled arm).
	addSeedPerRoute := func(pr proto.Message) {
		f.Helper()
		raw, err := proto.Marshal(pr)
		if err != nil {
			f.Fatalf("per-route seed marshal: %v", err)
		}
		f.Add(raw)
	}
	// Per-route seeds are added as raw bytes of ExtAuthzPerRoute
	// (the fuzzer fuzzes the ExtAuthz top-level; per-route parsing tested via unit tests).
	_ = addSeedPerRoute // suppress unused warning — we use per-route shapes below via full ExtAuthz seeds

	// (6) per-route disabled: true — exercised via the full ExtAuthz top-level
	// (the fuzzer targets New which parses ExtAuthz; ExtAuthzPerRoute is in the
	// per-route registry, not the filter config — seeds focus on what New sees).
	addSeed(&ext_authzv3.ExtAuthz{
		Services:            minHTTPService("http://127.0.0.1:9191/auth"),
		TransportApiVersion: corev3.ApiVersion_V3,
		// clear_route_cache exercises a commonly-set field.
		ClearRouteCache: true,
	})

	// (7) per-route disabled: false seed — we seed the per-route proto via a
	// standalone ExtAuthzPerRoute Any (passed as raw bytes via anypb trick below).
	// Since New only accepts ExtAuthz top-level, this seed exercises the
	// unmarshal-into-wrong-type PARSE-REJECT path in the fuzzer.
	{
		pr := &ext_authzv3.ExtAuthzPerRoute{
			Override: &ext_authzv3.ExtAuthzPerRoute_Disabled{Disabled: false},
		}
		raw, _ := proto.Marshal(pr)
		f.Add(raw) // Will unmarshal as ExtAuthz (garbled); exercises the fuzzer's wrong-type path.
	}

	// (8) per-route check_settings seed (same raw-bytes trick as (7)).
	{
		pr := &ext_authzv3.ExtAuthzPerRoute{
			Override: &ext_authzv3.ExtAuthzPerRoute_CheckSettings{
				CheckSettings: &ext_authzv3.CheckSettings{
					DisableRequestBodyBuffering: true,
				},
			},
		}
		raw, _ := proto.Marshal(pr)
		f.Add(raw)
	}

	// Exact matcher helper.
	exactMatcher := func(value string, ignoreCase bool) *matcherv3.StringMatcher {
		return &matcherv3.StringMatcher{
			MatchPattern: &matcherv3.StringMatcher_Exact{Exact: value},
			IgnoreCase:   ignoreCase,
		}
	}
	listMatcher := func(matchers ...*matcherv3.StringMatcher) *matcherv3.ListStringMatcher {
		return &matcherv3.ListStringMatcher{Patterns: matchers}
	}

	// (9) allowed_headers exact matcher.
	addSeed(&ext_authzv3.ExtAuthz{
		Services:            minHTTPService("http://127.0.0.1:9191/auth"),
		TransportApiVersion: corev3.ApiVersion_V3,
		AllowedHeaders:      listMatcher(exactMatcher("x-user-id", false)),
	})

	// (10) allowed_headers prefix matcher.
	addSeed(&ext_authzv3.ExtAuthz{
		Services:            minHTTPService("http://127.0.0.1:9191/auth"),
		TransportApiVersion: corev3.ApiVersion_V3,
		AllowedHeaders: listMatcher(&matcherv3.StringMatcher{
			MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "x-"},
		}),
	})

	// (11) allowed_headers suffix matcher.
	addSeed(&ext_authzv3.ExtAuthz{
		Services:            minHTTPService("http://127.0.0.1:9191/auth"),
		TransportApiVersion: corev3.ApiVersion_V3,
		AllowedHeaders: listMatcher(&matcherv3.StringMatcher{
			MatchPattern: &matcherv3.StringMatcher_Suffix{Suffix: "-token"},
		}),
	})

	// (12) allowed_headers contains matcher.
	addSeed(&ext_authzv3.ExtAuthz{
		Services:            minHTTPService("http://127.0.0.1:9191/auth"),
		TransportApiVersion: corev3.ApiVersion_V3,
		AllowedHeaders: listMatcher(&matcherv3.StringMatcher{
			MatchPattern: &matcherv3.StringMatcher_Contains{Contains: "auth"},
		}),
	})

	// (13) allowed_headers safe_regex matcher (RE2-compatible engine).
	addSeed(&ext_authzv3.ExtAuthz{
		Services:            minHTTPService("http://127.0.0.1:9191/auth"),
		TransportApiVersion: corev3.ApiVersion_V3,
		AllowedHeaders: listMatcher(&matcherv3.StringMatcher{
			MatchPattern: &matcherv3.StringMatcher_SafeRegex{
				SafeRegex: &matcherv3.RegexMatcher{
					Regex: `^x-auth-.*`,
					EngineType: &matcherv3.RegexMatcher_GoogleRe2{
						GoogleRe2: &matcherv3.RegexMatcher_GoogleRE2{},
					},
				},
			},
		}),
	})

	// (14) allowed_headers custom matcher — PARSE-REJECT path (nil Custom value
	// triggers the custom-arm with a nil TypedExtensionConfig; non-nil is a
	// compile error since the field type comes from a different proto package).
	addSeed(&ext_authzv3.ExtAuthz{
		Services:            minHTTPService("http://127.0.0.1:9191/auth"),
		TransportApiVersion: corev3.ApiVersion_V3,
		AllowedHeaders: listMatcher(&matcherv3.StringMatcher{
			MatchPattern: &matcherv3.StringMatcher_Custom{
				Custom: nil, // non-nil arm, nil TypedExtensionConfig — triggers PARSE-REJECT.
			},
		}),
	})

	// (15) disallowed_headers exact matcher (combined with allowed_headers).
	addSeed(&ext_authzv3.ExtAuthz{
		Services:            minHTTPService("http://127.0.0.1:9191/auth"),
		TransportApiVersion: corev3.ApiVersion_V3,
		AllowedHeaders:      listMatcher(exactMatcher("x-user-id", false)),
		DisallowedHeaders:   listMatcher(exactMatcher("x-internal", false)),
	})

	// (16) error-posture: failure_mode_allow + failure_mode_allow_header_add +
	// status_on_error 503.
	addSeed(&ext_authzv3.ExtAuthz{
		Services:                  minHTTPService("http://127.0.0.1:9191/auth"),
		TransportApiVersion:       corev3.ApiVersion_V3,
		FailureModeAllow:          true,
		FailureModeAllowHeaderAdd: true,
		StatusOnError:             &typev3.HttpStatus{Code: typev3.StatusCode_ServiceUnavailable},
	})

	// (17) Full http_service with authorization_request + authorization_response.
	addSeed(&ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_HttpService{
			HttpService: &ext_authzv3.HttpService{
				ServerUri: &corev3.HttpUri{
					Uri:     "http://127.0.0.1:9191/auth",
					Timeout: &durationpb.Duration{Seconds: 2},
				},
				PathPrefix: "/ext-authz",
				AuthorizationRequest: &ext_authzv3.AuthorizationRequest{
					HeadersToAdd: []*corev3.HeaderValue{
						{Key: "x-auth-source", Value: "envoy"},
					},
				},
				AuthorizationResponse: &ext_authzv3.AuthorizationResponse{
					AllowedUpstreamHeaders: listMatcher(exactMatcher("x-user-id", false)),
					AllowedClientHeaders:   listMatcher(exactMatcher("x-error-reason", false)),
				},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
		WithRequestBody: &ext_authzv3.BufferSettings{
			MaxRequestBytes:     4096,
			AllowPartialMessage: false,
		},
		AllowedHeaders:    listMatcher(exactMatcher("authorization", true)),
		ValidateMutations: true,
	})

	// (18) Non-V3 transport_api_version — PARSE-REJECT (ADR-0008).
	addSeed(&ext_authzv3.ExtAuthz{
		Services:            minHTTPService("http://127.0.0.1:9191/auth"),
		TransportApiVersion: corev3.ApiVersion_V2, //nolint:staticcheck // intentional: testing PARSE-REJECT of non-V3 transport API version
	})

	// (19) with_request_body allow_partial_message + pack_as_bytes.
	addSeed(&ext_authzv3.ExtAuthz{
		Services:            minHTTPService("http://127.0.0.1:9191/auth"),
		TransportApiVersion: corev3.ApiVersion_V3,
		WithRequestBody: &ext_authzv3.BufferSettings{
			MaxRequestBytes:     1024,
			AllowPartialMessage: true,
			PackAsBytes:         true,
		},
	})

	// (20) validate_mutations true with full field surface.
	addSeed(&ext_authzv3.ExtAuthz{
		Services:            minHTTPService("http://127.0.0.1:9191/auth"),
		TransportApiVersion: corev3.ApiVersion_V3,
		ValidateMutations:   true,
		ClearRouteCache:     true,
		FailureModeAllow:    false,
		StatusOnError:       &typev3.HttpStatus{Code: typev3.StatusCode_Forbidden},
	})

	// -------------------------------------------------------------------------
	// Phase 18.2 grpc_service corpus extension (Task 9 / SPEC §7.3).
	//
	// The 18.2 grpc_service mode flips seed (2) from PARSE-REJECT (18.1 wording)
	// to a real-but-unknown-cluster PARSE-REJECT path: the empty FactoryCtx has
	// no ClusterManager, so buildGRPCCheckFn lands the
	// "cluster manager not available" error. Each new seed below exercises a
	// distinct grpc_service surface — the structural contract (never-both-nil;
	// never-both-set; never-panic) holds on every PARSE-REJECT branch.
	// -------------------------------------------------------------------------

	// (21) grpc_service envoy_grpc valid cluster_name — PARSE-REJECT under the
	// empty FactoryCtx (cluster_manager nil); valid under a real FactoryCtx
	// (exercised in extauthz_test.go Group 10).
	addSeed(&ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_GrpcService{
			GrpcService: &corev3.GrpcService{
				TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{
						ClusterName: "c_authz_grpc",
					},
				},
				Timeout: &durationpb.Duration{Seconds: 1},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
	})

	// (22) grpc_service envoy_grpc empty cluster_name — PARSE-REJECT (PGV-mirror
	// `min_len: 1` per SPEC §6.5).
	addSeed(&ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_GrpcService{
			GrpcService: &corev3.GrpcService{
				TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{
						ClusterName: "",
					},
				},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
	})

	// (23) grpc_service envoy_grpc unknown cluster_name — PARSE-REJECT (cluster
	// not in the cluster manager; under empty FactoryCtx the nil-mgr branch
	// fires first, which is equally a PARSE-REJECT contract preserver).
	addSeed(&ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_GrpcService{
			GrpcService: &corev3.GrpcService{
				TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{
						ClusterName: "this-cluster-does-not-exist",
					},
				},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
	})

	// (24) grpc_service google_grpc arm — PARSE-REJECT envoy-go-strict per
	// ADR-0157 AMENDMENT.
	addSeed(&ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_GrpcService{
			GrpcService: &corev3.GrpcService{
				TargetSpecifier: &corev3.GrpcService_GoogleGrpc_{
					GoogleGrpc: &corev3.GrpcService_GoogleGrpc{
						TargetUri:  "auth.example.com:443",
						StatPrefix: "ext_authz_grpc",
					},
				},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
	})

	// (25) grpc_service envoy_grpc with initial_metadata populated — silent-
	// ignored per SPEC §2.6 + §8 item 2; the fuzzer asserts no panic.
	addSeed(&ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_GrpcService{
			GrpcService: &corev3.GrpcService{
				TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{
						ClusterName: "c_authz_grpc",
					},
				},
				InitialMetadata: []*corev3.HeaderValue{
					{Key: "x-tenant", Value: "blue"},
					{Key: "x-trace-id", Value: "abc-123"},
				},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
	})

	// (26) grpc_service envoy_grpc with retry_policy populated — silent-ignored
	// per SPEC §2.6 + §8 item 3. The outer GrpcService.retry_policy carries
	// num_retries + retry_on; envoy-go's buildGRPCCheckFn neither reads nor
	// rejects the field. The fuzzer asserts no panic.
	addSeed(&ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_GrpcService{
			GrpcService: &corev3.GrpcService{
				TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{
						ClusterName: "c_authz_grpc",
					},
				},
				RetryPolicy: &corev3.RetryPolicy{
					NumRetries: &wrapperspb.UInt32Value{Value: 3},
					RetryOn:    "5xx",
				},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V3,
	})

	// (27) grpc_service envoy_grpc + non-V3 transport_api_version — PARSE-REJECT
	// per ADR-0008 (transport_api_version V3 only).
	addSeed(&ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_GrpcService{
			GrpcService: &corev3.GrpcService{
				TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{
						ClusterName: "c_authz_grpc",
					},
				},
			},
		},
		TransportApiVersion: corev3.ApiVersion_V2, //nolint:staticcheck // intentional: PARSE-REJECT of non-V3 transport API version
	})

	// (28) grpc_service envoy_grpc + with_request_body + grpc-mode booleans —
	// exercises the full grpc-mode config surface (encode_raw_headers,
	// include_peer_certificate, include_tls_session — bool fields exist on
	// ExtAuthz). Either path lands a coherent shape; the fuzz body asserts the
	// structural contract.
	addSeed(&ext_authzv3.ExtAuthz{
		Services: &ext_authzv3.ExtAuthz_GrpcService{
			GrpcService: &corev3.GrpcService{
				TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{
						ClusterName: "c_authz_grpc",
					},
				},
				Timeout: &durationpb.Duration{Seconds: 2},
			},
		},
		TransportApiVersion:    corev3.ApiVersion_V3,
		WithRequestBody:        &ext_authzv3.BufferSettings{MaxRequestBytes: 1024, PackAsBytes: true},
		IncludePeerCertificate: true,
		IncludeTlsSession:      true,
		EncodeRawHeaders:       true,
		ValidateMutations:      true,
	})

	// Empty bytes — Unmarshal succeeds to zero-value ExtAuthz; the empty-services
	// PARSE-REJECT fires. Contract holds.
	f.Add([]byte{})

	// -------------------------------------------------------------------------
	// Fuzz body: structural contract assertions only.
	// -------------------------------------------------------------------------
	f.Fuzz(func(t *testing.T, raw []byte) {
		// Empty FactoryCtx (no Stats registry) per phase-14/15/16/17 precedent:
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

// FuzzCheckResponseMapping fuzzes arbitrary bytes as `*authv3.CheckResponse`
// proto payloads + `proto.Unmarshal`s them + drives `mapGRPCResponse` per the
// 6-row dispatch table at SPEC §6.7 + §7.3. 23rd fuzzer overall (phases 02–18.1
// contributed 22).
//
// Asserts the structural contract of `mapGRPCResponse`:
//
//   - The function never panics on any proto.Unmarshal-able `CheckResponse`.
//   - The returned `checkDisposition.class` is a valid enum value
//     (dispAllow | dispDeny | dispError | dispInvalid).
//   - On dispDeny, `denyStatus` is non-zero (the proto-zero default fires the
//     SPEC §6.7 fallback to 403). NOTE: no upper bound — the proto wire format
//     for `typev3.StatusCode` admits arbitrary int32 values (the proto enum is
//     unvalidated at the wire boundary); buildDenyDispositionGRPC carries the
//     value verbatim. The auth-server contract is to populate standard HTTP
//     status codes; surfacing whatever the auth-server emitted is the
//     intentional envoy-go-strict discipline.
//   - `denyHeaders` pass `validateMutationHeaders` when validate_mutations:true
//     (the validate_mutations path drives dispInvalid on a violation, so a
//     surviving dispDeny implies the headers passed validation — the fuzzer
//     asserts the contract).
//
// Seed corpus per the PLAN File-structure spec (9 variants covering each
// `mapGRPCResponse` truth-table case + boundaries):
//
//	(0)  valid OK + OkResponse{} — empty allow
//	(1)  OK + OkResponse with mutations (each append_action arm)
//	(2)  OK + DeniedResponse — structurally inconsistent → dispError
//	(3)  non-OK + DeniedResponse with denyStatus 401
//	(4)  non-OK + DeniedResponse with denyStatus 403
//	(5)  non-OK + DeniedResponse with denyStatus 500
//	(6)  non-OK + DeniedResponse with denyStatus 0 (proto zero) → 403 default
//	(7)  non-OK + OkResponse — structurally inconsistent → dispError
//	(8)  empty CheckResponse{} — defensive allow row
//	(9)  oversized header values (~64 KiB) — exercises the validate_mutations
//	     value-length boundary
//	(10) pseudo-header `:authority` in OkResponse mutations → dispInvalid under
//	     validate_mutations:true
//
// 30s/seed runtime envelope per ADR-0018 short-mode CI policy.
func FuzzCheckResponseMapping(f *testing.F) {
	addSeed := func(msg proto.Message) {
		f.Helper()
		raw, err := proto.Marshal(msg)
		if err != nil {
			f.Fatalf("seed marshal: %v", err)
		}
		f.Add(raw)
	}

	// (0) Valid OK + empty OkResponse{} — bare allow.
	addSeed(&authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: 0},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{},
		},
	})

	// (1) OK + OkResponse with mutations across each append_action arm (D5).
	addSeed(&authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: 0},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{
				Headers: []*corev3.HeaderValueOption{
					{
						Header:       &corev3.HeaderValue{Key: "x-set-default", Value: "v1"},
						AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
					},
					{
						Header:       &corev3.HeaderValue{Key: "x-append", Value: "v2"},
						AppendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
					},
					{
						Header:       &corev3.HeaderValue{Key: "x-overwrite-only", Value: "v3"},
						AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS,
					},
					{
						Header:       &corev3.HeaderValue{Key: "x-add-if-absent", Value: "v4"},
						AppendAction: corev3.HeaderValueOption_ADD_IF_ABSENT,
					},
				},
				HeadersToRemove: []string{"x-internal", "x-secret"},
			},
		},
	})

	// (2) OK + DeniedResponse — structurally inconsistent per SPEC §6.7
	// commentary; envoy-go-strict treats as dispError.
	addSeed(&authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: 0},
		HttpResponse: &authv3.CheckResponse_DeniedResponse{
			DeniedResponse: &authv3.DeniedHttpResponse{
				Status: &typev3.HttpStatus{Code: typev3.StatusCode_Forbidden},
				Body:   "forbidden",
			},
		},
	})

	// (3) non-OK + DeniedResponse with denyStatus 401 — canonical deny.
	addSeed(&authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: 16 /*UNAUTHENTICATED*/},
		HttpResponse: &authv3.CheckResponse_DeniedResponse{
			DeniedResponse: &authv3.DeniedHttpResponse{
				Status: &typev3.HttpStatus{Code: typev3.StatusCode_Unauthorized},
				Body:   "unauthenticated",
				Headers: []*corev3.HeaderValueOption{
					{Header: &corev3.HeaderValue{Key: "www-authenticate", Value: "Bearer realm=\"x\""}},
				},
			},
		},
	})

	// (4) non-OK + DeniedResponse with denyStatus 403.
	addSeed(&authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: 7 /*PERMISSION_DENIED*/},
		HttpResponse: &authv3.CheckResponse_DeniedResponse{
			DeniedResponse: &authv3.DeniedHttpResponse{
				Status: &typev3.HttpStatus{Code: typev3.StatusCode_Forbidden},
				Body:   "forbidden",
			},
		},
	})

	// (5) non-OK + DeniedResponse with denyStatus 500.
	addSeed(&authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: 13 /*INTERNAL*/},
		HttpResponse: &authv3.CheckResponse_DeniedResponse{
			DeniedResponse: &authv3.DeniedHttpResponse{
				Status: &typev3.HttpStatus{Code: typev3.StatusCode_InternalServerError},
				Body:   "internal error",
			},
		},
	})

	// (6) non-OK + DeniedResponse with proto-zero status — buildDenyDispositionGRPC
	// defaults to 403 per SPEC §6.7.
	addSeed(&authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: 7},
		HttpResponse: &authv3.CheckResponse_DeniedResponse{
			DeniedResponse: &authv3.DeniedHttpResponse{
				// Status omitted → zero → default 403 per SPEC §6.7.
				Body: "no status",
			},
		},
	})

	// (7) non-OK + OkResponse — structurally inconsistent per SPEC §6.7
	// commentary; envoy-go-strict treats as dispError.
	addSeed(&authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: 7},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{},
		},
	})

	// (8) Empty CheckResponse{} — the defensive allow row (status-zero + nil
	// HttpResponse oneof; mapGRPCResponse returns dispAllow).
	addSeed(&authv3.CheckResponse{})

	// (9) Oversized header value (~16 KiB) on an OkResponse mutation — the
	// validate_mutations path is value-character-only (no length cap per D7);
	// this seed exercises the no-length-cap discipline + the value-character
	// validation surface against long values.
	{
		longVal := make([]byte, 16*1024)
		for i := range longVal {
			longVal[i] = 'a' + byte(i%26)
		}
		addSeed(&authv3.CheckResponse{
			Status: &rpcstatus.Status{Code: 0},
			HttpResponse: &authv3.CheckResponse_OkResponse{
				OkResponse: &authv3.OkHttpResponse{
					Headers: []*corev3.HeaderValueOption{
						{Header: &corev3.HeaderValue{Key: "x-big-value", Value: string(longVal)}},
					},
				},
			},
		})
	}

	// (10) Pseudo-header `:authority` in OkResponse mutations — under
	// validate_mutations:true this drives dispInvalid per D7 (the fuzzer
	// asserts the contract: the validate_mutations branch rejects the
	// :-prefixed name).
	addSeed(&authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: 0},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{
				Headers: []*corev3.HeaderValueOption{
					{Header: &corev3.HeaderValue{Key: ":authority", Value: "evil.example.com"}},
				},
			},
		},
	})

	// Empty bytes — Unmarshal succeeds to zero-value CheckResponse{}; the
	// defensive-allow row fires. Contract holds.
	f.Add([]byte{})

	// -------------------------------------------------------------------------
	// Fuzz body: structural contract assertions only.
	// -------------------------------------------------------------------------
	f.Fuzz(func(t *testing.T, b []byte) {
		var resp authv3.CheckResponse
		if err := proto.Unmarshal(b, &resp); err != nil {
			return // skip non-proto inputs
		}
		// Drive both validate_mutations branches: the contract is the same
		// (mapGRPCResponse never panics; produces a coherent disposition).
		for _, validateMutations := range []bool{false, true} {
			disp := mapGRPCResponse(&resp, validateMutations)

			// (a) class must be a valid enum value.
			switch disp.class {
			case dispAllow, dispDeny, dispError, dispInvalid:
				// ok
			default:
				t.Fatalf("disp.class %d not in {allow, deny, error, invalid}", disp.class)
			}

			// (b) on deny, denyStatus must be non-zero (proto-zero defaults
			// to 403 per SPEC §6.7 in buildDenyDispositionGRPC; the empty-
			// HttpResponse-oneof + non-zero-status row also returns 403).
			// No upper bound: the proto wire format admits arbitrary int32 values
			// for typev3.StatusCode; envoy-go-strict carries the value verbatim.
			if disp.class == dispDeny && disp.denyStatus == 0 {
				t.Fatalf("dispDeny denyStatus = 0 — buildDenyDispositionGRPC must default to 403 per SPEC §6.7")
			}

			// (c) denyHeaders pass validateMutationHeaders when
			// validate_mutations:true AND the disposition stayed at dispDeny.
			// (A violation would have driven dispInvalid; reaching dispDeny
			// with validate_mutations:true implies headers passed.)
			if validateMutations && disp.class == dispDeny {
				if err := validateMutationHeaders(disp.denyHeaders); err != nil {
					t.Fatalf("dispDeny denyHeaders failed validateMutationHeaders post-mapGRPCResponse: %v", err)
				}
			}
			// Same contract for the allow-path upstreamSet/upstreamApp.
			if validateMutations && disp.class == dispAllow {
				if err := validateMutationHeaders(disp.upstreamSet); err != nil {
					t.Fatalf("dispAllow upstreamSet failed validateMutationHeaders post-mapGRPCResponse: %v", err)
				}
				if err := validateMutationHeaders(disp.upstreamApp); err != nil {
					t.Fatalf("dispAllow upstreamApp failed validateMutationHeaders post-mapGRPCResponse: %v", err)
				}
			}
		}
	})
}
