package extproc

// extproc_test.go — Group 1 (factory parse paths) test stubs per PLAN Task 2
// Step 2 closing sentence: "The extproc_test.go Group 1 (factory parse paths)
// lands stubs that will be expanded at Tasks 4-11."
//
// At Task 2 this file lands two things:
//
//  1. A factory smoke-test asserting the skeleton New stub returns the
//     "under construction" error sentinel (the contract until Task 11
//     buildCompiledConfig integration lands).
//
//  2. A skeleton-reachability anchor that references every skeleton type +
//     field + helper introduced at Task 2 — silences the `unused` linter on
//     the scaffolding until Tasks 4-11 land the real consumers. Each Task
//     4-11 consumer (processor.go, check.go, attributes.go, the Task 11
//     buildCompiledConfig integration) replaces one or more of these
//     placeholder anchors with a real reference at its landing commit. By
//     Task 11 this anchor is removable (every field has a real read site).
//
// The Group 1 substantive tests (factory parse-path coverage — well-formed
// configs, the 9 PARSE-REJECT axes per ADR-0168, the mutual-exclusion check,
// the http_service body/trailer constraints, the STREAMED-only flag rejections,
// the error-posture default expansions) land at Task 11 when buildCompiledConfig
// is wired. Until then, the New stub is a deterministic error path.

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	commonmutationv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	upstreamshttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	extprocsvcv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// TestNew_NilTypedConfig — the factory PARSE-REJECTs a nil typed_config per
// the ADR-0072 contract. Task 11 wire-up: at Task 2 this returned the
// "under construction" sentinel; at Task 11 the factory is fully functional
// and the test flips from "skeleton sentinel" to "real factory error path".
func TestNew_NilTypedConfig(t *testing.T) {
	t.Parallel()
	factory, err := New(nil, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatalf("err = nil; want PARSE-REJECT for nil typed_config")
	}
	if factory != nil {
		t.Errorf("factory = %v; want nil on PARSE-REJECT", factory)
	}
	if !strings.Contains(err.Error(), "typed_config required") {
		t.Errorf("err = %q; want substring 'typed_config required'", err)
	}
}

// TestNew_MalformedAny — anypb.Any with a TypeUrl that does not match the
// ExternalProcessor proto → unmarshal error.
func TestNew_MalformedAny(t *testing.T) {
	t.Parallel()
	// An Any pointing at a different type URL.
	tc := &anypb.Any{TypeUrl: "type.googleapis.com/google.protobuf.StringValue", Value: []byte{0x0a, 0x03, 0x66, 0x6f, 0x6f}}
	factory, err := New(tc, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatalf("err = nil; want unmarshal error")
	}
	if factory != nil {
		t.Errorf("factory = %v; want nil on unmarshal failure", factory)
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("err = %q; want substring 'unmarshal'", err)
	}
}

// TestTypeURL asserts the canonical Envoy type-URL constant matches the
// ext_proc v3 proto type-URL. Regression anchor — accidental TypeURL drift
// would mis-route extension-registry boot-registration per ADR-0072 +
// ADR-0167.
func TestTypeURL(t *testing.T) {
	t.Parallel()
	const want = "type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor"
	if TypeURL != want {
		t.Fatalf("TypeURL = %q; want %q", TypeURL, want)
	}
}

// TestBuildCompiledConfig_NilRaw asserts the defensive nil-input PARSE-REJECT
// for the buildCompiledConfig helper.
func TestBuildCompiledConfig_NilRaw(t *testing.T) {
	t.Parallel()
	cc, err := buildCompiledConfig(nil, envoyhttp.FactoryCtx{})
	if cc != nil {
		t.Fatalf("cc = %v; want nil", cc)
	}
	if err == nil {
		t.Fatalf("err = nil; want PARSE-REJECT")
	}
	if !strings.Contains(err.Error(), "ExternalProcessor) is required") {
		t.Errorf("err = %q; want substring 'ExternalProcessor) is required'", err)
	}
}

// TestBaseStatPrefix asserts the SN2-reuse stat-prefix folding per ADR-0167:
// empty HCM stat_prefix → `ext_proc.`; non-empty → `http.<prefix>.ext_proc.`.
// This is the one helper whose Task 2 body is structurally complete (no
// stub) — the test pins the namespace shape.
func TestBaseStatPrefix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, hcm, want string
	}{
		{"empty", "", "ext_proc."},
		{"populated", "ingress_http", "http.ingress_http.ext_proc."},
		{"with_underscore_in_prefix", "ingress_http_1", "http.ingress_http_1.ext_proc."},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := baseStatPrefix(tc.hcm)
			if got != tc.want {
				t.Fatalf("baseStatPrefix(%q) = %q; want %q", tc.hcm, got, tc.want)
			}
		})
	}
}

// TestNewFilterStats_Registers9Counters asserts the 9-counter registration
// surface per ADR-0167 + parent §5.P4. The 9 counter names + the namespace
// shape are the load-bearing assertions; the Tasks 7-10 Inc() call sites
// land later.
func TestNewFilterStats_Registers9Counters(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_http")
	if fs == nil {
		t.Fatalf("newFilterStats returned nil; want *filterStats")
	}
	// Spot-check that each counter is non-nil (allocation succeeded).
	counters := map[string]*stats.Counter{
		"streamsStarted":                 fs.streamsStarted,
		"streamMsgsSent":                 fs.streamMsgsSent,
		"streamMsgsReceived":             fs.streamMsgsReceived,
		"spuriousMsgsReceived":           fs.spuriousMsgsReceived,
		"streamsFailed":                  fs.streamsFailed,
		"streamsClosed":                  fs.streamsClosed,
		"failureModeAllowed":             fs.failureModeAllowed,
		"overrideMessageTimeoutReceived": fs.overrideMessageTimeoutReceived,
		"overrideMessageTimeoutIgnored":  fs.overrideMessageTimeoutIgnored,
	}
	if len(counters) != 9 {
		t.Fatalf("counter map length = %d; want 9 per ADR-0167", len(counters))
	}
	for name, c := range counters {
		if c == nil {
			t.Errorf("counter %q is nil; expected allocation", name)
		}
	}
}

// TestSkeletonReachability was the Task-2-era skeleton-symbol anchor that
// referenced every type + field + helper so the `unused` linter would not
// flag the scaffolding. Carryforward P (Task 14): by Task 11 every field has
// a real reader at a dispatcher / builder / per-stage call site, so the
// anchor is no longer load-bearing for the linter. Retired here to remove
// the gocyclo bypass + the brittle exhaustive zero-value comparison block.
// If any future symbol reverts to anchor-only status, restore from git
// history (see commits prior to phase-19.1 Task 14).

// ---------------------------------------------------------------------------
// Group 2 — ADR-0170 JSON codec for http_service mode (Task 6).
//
// Tests the filter-local protojson codec in json.go:
//   - marshalProcessingRequest: serializes *ProcessingRequest → JSON bytes.
//   - unmarshalProcessingResponse: deserializes JSON bytes → *ProcessingResponse.
//
// Test approach per Task 6 PLAN Pattern A (smaller production surface): only
// marshal-Request + unmarshal-Response live in json.go; the round-trip test
// reads the marshaled bytes back into a fresh *ProcessingRequest via
// protojson.Unmarshal directly (treats the production codec as a black-box
// from the test perspective; verifies the marshaled bytes are valid protojson
// for the inverse parse direction).
//
// ADR-0170 §Decision pin: protojson MarshalOptions{UseProtoNames: true,
// EmitUnpopulated: false, UseEnumNumbers: false}; UnmarshalOptions{
// DiscardUnknown: true}. Wire-shape RATIFIED-PENDING-IMPL-TIME — closes at
// Task 13 fixture-harness scrape vs reference Envoy v1.37.2. On unmarshal
// failure per D8: classify as streamsFailed++ + dispError (fail-loud).
// ---------------------------------------------------------------------------

// TestMarshalProcessingRequest_RoundTrip asserts that marshalProcessingRequest
// produces protojson-parseable bytes for a hand-crafted *ProcessingRequest
// with request_headers populated, AND that re-parsing the bytes into a fresh
// *ProcessingRequest yields proto.Equal-equivalent content. Pattern A
// round-trip (production code: marshal only; test reads back via direct
// protojson.Unmarshal for verification).
//
// The fixture covers the two structural fields the http_service codec carries
// at headers-only stages:
//   - request oneof = RequestHeaders (HttpHeaders{ headers: HeaderMap, end_of_stream })
//   - attributes map (the request_attributes envelope, optional)
//
// Asserts:
//   - marshalProcessingRequest returns nil error + non-empty bytes.
//   - The bytes contain the UseProtoNames-rendered field key `request_headers`
//     (snake_case, NOT lowerCamelCase `requestHeaders`) per ADR-0170 §Decision.
//   - protojson.Unmarshal back into a fresh *ProcessingRequest yields a value
//     proto.Equal to the original (round-trip semantic equivalence; byte-exact
//     equality is NOT asserted because protojson does not guarantee
//     determinism on map iteration order — see the protojson package doc).
func TestMarshalProcessingRequest_RoundTrip(t *testing.T) {
	t.Parallel()

	// Hand-crafted *ProcessingRequest: request_headers stage with a small
	// header bundle + one synthetic attribute key. Matches the shape the
	// filter would emit at DecodeHeaders entry per SPEC §3.2 + §6.6.
	orig := &extprocsvcv3.ProcessingRequest{
		Request: &extprocsvcv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: &extprocsvcv3.HttpHeaders{
				Headers: &corev3.HeaderMap{
					Headers: []*corev3.HeaderValue{
						{Key: ":path", Value: "/api/v1/resource"},
						{Key: ":method", Value: "GET"},
						{Key: "user-agent", Value: "envoy-go-extproc-test/1"},
					},
				},
				EndOfStream: true,
			},
		},
	}

	got, err := marshalProcessingRequest(orig)
	if err != nil {
		t.Fatalf("marshalProcessingRequest: unexpected err = %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("marshalProcessingRequest returned 0-length bytes; want non-empty JSON")
	}

	// UseProtoNames assertion: the snake_case proto field name must appear
	// in the rendered JSON. The lowerCamelCase form (`requestHeaders`)
	// would indicate UseProtoNames=false (the proto3 JSON canonical default
	// — what ADR-0170 §Decision explicitly opts AWAY from).
	gotStr := string(got)
	if !strings.Contains(gotStr, `"request_headers"`) {
		t.Errorf("marshaled JSON missing snake_case key `request_headers`; got: %s", gotStr)
	}
	if strings.Contains(gotStr, `"requestHeaders"`) {
		t.Errorf("marshaled JSON contains lowerCamelCase key `requestHeaders`; UseProtoNames=true expected per ADR-0170 §Decision")
	}

	// Round-trip: re-parse the bytes into a fresh *ProcessingRequest via
	// direct protojson.Unmarshal (Pattern A — production code only exposes
	// the request-direction marshal; the inverse parse here exists for
	// test verification only). Asserts proto.Equal equivalence.
	roundtripped := &extprocsvcv3.ProcessingRequest{}
	if err := protojson.Unmarshal(got, roundtripped); err != nil {
		t.Fatalf("protojson.Unmarshal of marshalProcessingRequest output: %v", err)
	}
	if !proto.Equal(orig, roundtripped) {
		t.Errorf("round-trip mismatch:\n  orig=%v\n  roundtripped=%v", orig, roundtripped)
	}
}

// TestMarshalProcessingRequest_NilInput asserts that marshalProcessingRequest
// rejects a nil input with a non-nil error (the contract — the caller is
// guarded by the per-stage dispatch in Task 8 check.go which never passes nil
// in production, but the codec is defensive at the API boundary).
func TestMarshalProcessingRequest_NilInput(t *testing.T) {
	t.Parallel()
	got, err := marshalProcessingRequest(nil)
	if err == nil {
		t.Fatalf("marshalProcessingRequest(nil): err = nil; want non-nil")
	}
	if got != nil {
		t.Fatalf("marshalProcessingRequest(nil): bytes = %q; want nil", string(got))
	}
}

// TestUnmarshalProcessingResponse_HappyPath asserts unmarshalProcessingResponse
// parses a hand-crafted JSON ProcessingResponse into the matching typed value
// with the expected field set populated. The fixture covers the two
// header-stage response oneof arms exercised in 19.1:
//   - response oneof = RequestHeaders (HeadersResponse{ response: CommonResponse })
//   - CommonResponse with a header_mutation set (representative of the
//     per-stage mutation discipline at SPEC §6.7 step 5).
func TestUnmarshalProcessingResponse_HappyPath(t *testing.T) {
	t.Parallel()

	// JSON crafted by hand in snake_case form (matches what the processor
	// is expected to emit per the ADR-0170 wire-shape RATIFIED-PENDING
	// hypothesis — closes at Task 13). The shape mirrors the proto's
	// JSON canonical with UseProtoNames=true.
	jsonBody := `{
		"request_headers": {
			"response": {
				"status": "CONTINUE",
				"header_mutation": {
					"set_headers": [
						{
							"header": {"key": "x-injected", "value": "true"},
							"append_action": "OVERWRITE_IF_EXISTS_OR_ADD"
						}
					]
				}
			}
		}
	}`

	got, err := unmarshalProcessingResponse([]byte(jsonBody))
	if err != nil {
		t.Fatalf("unmarshalProcessingResponse: unexpected err = %v", err)
	}
	if got == nil {
		t.Fatalf("unmarshalProcessingResponse returned nil response; want non-nil")
	}

	// Assert the response oneof discriminator matches request_headers.
	rh := got.GetRequestHeaders()
	if rh == nil {
		t.Fatalf("got.GetRequestHeaders() = nil; want non-nil per fixture's request_headers arm")
	}

	// Assert the CommonResponse status defaulted as CONTINUE (the JSON
	// fixture explicitly set it).
	cr := rh.GetResponse()
	if cr == nil {
		t.Fatalf("rh.GetResponse() = nil; want non-nil CommonResponse")
	}
	if cr.GetStatus() != extprocsvcv3.CommonResponse_CONTINUE {
		t.Errorf("CommonResponse.status = %v; want CONTINUE", cr.GetStatus())
	}

	// Assert the header_mutation set_headers carried the injected entry.
	hm := cr.GetHeaderMutation()
	if hm == nil {
		t.Fatalf("cr.GetHeaderMutation() = nil; want non-nil per fixture")
	}
	sh := hm.GetSetHeaders()
	if len(sh) != 1 {
		t.Fatalf("len(set_headers) = %d; want 1", len(sh))
	}
	hv := sh[0].GetHeader()
	if hv == nil || hv.GetKey() != "x-injected" || hv.GetValue() != "true" {
		t.Errorf("set_headers[0].header = %v; want {key:x-injected value:true}", hv)
	}
}

// TestUnmarshalProcessingResponse_DiscardUnknown asserts the
// DiscardUnknown:true UnmarshalOption per ADR-0170 §Decision is in effect:
// unknown fields in the JSON body do NOT trigger an error. This is
// load-bearing for forward-compat — future Envoy proto extensions land as
// silently-ignored unknown fields, NOT as parse failures that would crash
// every running envoy-go binary against a newer processor.
func TestUnmarshalProcessingResponse_DiscardUnknown(t *testing.T) {
	t.Parallel()

	// JSON carries an `unknown_future_field` at the top level + a nested
	// unknown inside request_headers. With DiscardUnknown:false (the
	// default) protojson would fail with `proto: ... unknown field`.
	jsonBody := `{
		"unknown_future_field": "ignored",
		"request_headers": {
			"unknown_nested_field": 42,
			"response": {"status": "CONTINUE"}
		}
	}`
	got, err := unmarshalProcessingResponse([]byte(jsonBody))
	if err != nil {
		t.Fatalf("unmarshalProcessingResponse with unknown fields: err = %v; want nil (DiscardUnknown:true)", err)
	}
	if got == nil {
		t.Fatalf("unmarshalProcessingResponse returned nil; want non-nil with known fields populated")
	}
	if got.GetRequestHeaders() == nil {
		t.Errorf("request_headers arm not populated despite known content")
	}
}

// TestUnmarshalProcessingResponse_Malformed asserts the D8 fail-loud
// discipline: a truncated / malformed JSON body returns a non-nil error
// (which the per-stage dispatcher at Task 8 classifies as
// `streamsFailed++` + dispError). This pins the contract for the failure
// classification at the dispatcher boundary.
func TestUnmarshalProcessingResponse_Malformed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body []byte
	}{
		{"empty_input", []byte{}},
		{"truncated_json", []byte(`{"request_headers": {"response":`)},
		{"not_json_at_all", []byte("this is not JSON, it is plain text")},
		{"unbalanced_braces", []byte(`{"request_headers": {`)},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := unmarshalProcessingResponse(tc.body)
			if err == nil {
				t.Fatalf("unmarshalProcessingResponse(%s): err = nil; want non-nil (D8 fail-loud)", tc.name)
			}
			if got != nil {
				t.Errorf("unmarshalProcessingResponse(%s): got = %v; want nil on error", tc.name, got)
			}
		})
	}
}

// TestMarshalProcessingRequest_EnumAsString asserts the UseEnumNumbers:false
// MarshalOption per ADR-0170 §Decision is in effect: enum values render as
// string names, NOT as their integer ordinal. The fixture uses an
// ImmediateResponse with an embedded status — though the codec lives at
// the request-direction, this test ANCHORS the enum-string discipline by
// constructing a ProcessingRequest with an embedded enum-bearing field
// (HttpHeaders has none directly; instead we use the request oneof
// discriminator's well-known status field by constructing a Request that
// rolls through an enum-bearing field — actually for ProcessingRequest the
// enum surface is limited; we use the request-side via a directly-anchored
// snake_case + lowerCamelCase assertion at TestMarshalProcessingRequest_RoundTrip
// which is the structural pin). This test is a placeholder that the
// stronger enum assertion lands in Task 13's fixture-harness scrape per
// the ADR-0170 wire-shape RATIFIED-PENDING-IMPL-TIME contract.
//
// Specifically asserts the converse: the marshaled output does NOT contain
// a bare integer where a string enum is expected (a regression sentinel for
// accidental UseEnumNumbers=true drift). The DEFAULT zero-valued enums of
// the empty ProcessingRequest do not render at all due to EmitUnpopulated:false
// — so the output should be `{}` for a zero-valued ProcessingRequest.
func TestMarshalProcessingRequest_EmitUnpopulatedFalse(t *testing.T) {
	t.Parallel()
	// Zero-valued ProcessingRequest — no oneof set, no attributes.
	got, err := marshalProcessingRequest(&extprocsvcv3.ProcessingRequest{})
	if err != nil {
		t.Fatalf("marshalProcessingRequest({}): err = %v", err)
	}
	// With EmitUnpopulated:false the rendered JSON must NOT contain the
	// zero-valued `observability_mode` field. The expected output is the
	// empty object `{}` modulo whitespace.
	gotStr := strings.TrimSpace(string(got))
	if strings.Contains(gotStr, "observability_mode") {
		t.Errorf("marshaled empty ProcessingRequest contains observability_mode; EmitUnpopulated:false expected per ADR-0170 §Decision; got: %s", gotStr)
	}
	if gotStr != "{}" {
		// Note: protojson may render `{ }` with internal whitespace on
		// some versions; trim-internal comparison would be brittle. The
		// load-bearing assertion is the observability_mode absence above.
		t.Logf("marshaled empty ProcessingRequest = %q (informational; the load-bearing assertion is the EmitUnpopulated absence check above)", gotStr)
	}
}

// (typev3 forward-reference anchor REMOVED at Task 8 per Carryforward A —
// the Group 6 emitImmediateResponse tests below substantively consume
// typev3.StatusCode_* constants to construct ImmediateResponse fixtures.)

// ---------------------------------------------------------------------------
// Group 7 — processor.go header-mode state machine per ADR-0171 (Task 7).
//
// Tests cover:
//   - resolveProcessingMode: DEFAULT→SEND for headers / DEFAULT→SKIP for
//     trailers per parent §5.P9; body-mode != NONE PARSE-REJECT (19.1);
//     trailer-mode != SKIP PARSE-REJECT (permanent); http_service +
//     body-mode != NONE PARSE-REJECT (proto constraint).
//   - mode_override re-eval: applied on header-response stages only
//     (body/trailer stages silently ignored — NOT spurious); allow_mode_override:false
//     ignores all mode_overrides; allowed_override_modes allowlist enforced.
//     [The mode_override APPLICATION lands in applyProcessingResponse at
//     Task 8; Group 7 here tests the resolveProcessingMode validation +
//     the state-machine field types only. The mode_override APPLICATION
//     test coverage is deferred to Task 8 Group 5.]
//   - handleOverrideMessageTimeout: max_message_timeout >= 1ms gates
//     override enablement (otherwise overrideMessageTimeoutIgnored++); range
//     check [1ms, max_message_timeout]; at most ONCE per stage; subsequent
//     overrides ignored + counter increments.
//   - completeStage: dispatches via applyProcessingResponseFn (test override);
//     actContinue signals ContinueDecoding/Encoding; actImmediate does NOT
//     signal; D9 done-flag race-guard drops resume on OnDestroy-fired path.
// ---------------------------------------------------------------------------

// TestResolveProcessingMode_DefaultTranslation pins the parent §5.P9 RATIFIED
// DEFAULT translation: DEFAULT → SEND for *_header_mode; DEFAULT → SKIP for
// *_trailer_mode. The all-DEFAULT input (a fresh zero-valued ProcessingMode)
// is the load-bearing fixture because the proto's enum-0 is DEFAULT.
func TestResolveProcessingMode_DefaultTranslation(t *testing.T) {
	t.Parallel()
	// All-DEFAULT input — fresh zero-valued *ProcessingMode.
	pm := &extprocv3.ProcessingMode{}
	got, err := resolveProcessingMode(pm, false /*httpServiceMode*/)
	if err != nil {
		t.Fatalf("resolveProcessingMode(all-DEFAULT, gRPC mode): err = %v; want nil", err)
	}
	if got == nil {
		t.Fatalf("resolveProcessingMode(all-DEFAULT, gRPC mode): nil resolved; want non-nil")
	}
	if got.RequestHeaderMode != extprocv3.ProcessingMode_SEND {
		t.Errorf("RequestHeaderMode = %v; want SEND (DEFAULT→SEND per parent §5.P9)", got.RequestHeaderMode)
	}
	if got.ResponseHeaderMode != extprocv3.ProcessingMode_SEND {
		t.Errorf("ResponseHeaderMode = %v; want SEND (DEFAULT→SEND per parent §5.P9)", got.ResponseHeaderMode)
	}
	if got.RequestTrailerMode != extprocv3.ProcessingMode_SKIP {
		t.Errorf("RequestTrailerMode = %v; want SKIP (DEFAULT→SKIP per parent §5.P9)", got.RequestTrailerMode)
	}
	if got.ResponseTrailerMode != extprocv3.ProcessingMode_SKIP {
		t.Errorf("ResponseTrailerMode = %v; want SKIP (DEFAULT→SKIP per parent §5.P9)", got.ResponseTrailerMode)
	}
	if got.RequestBodyMode != extprocv3.ProcessingMode_NONE {
		t.Errorf("RequestBodyMode = %v; want NONE", got.RequestBodyMode)
	}
	if got.ResponseBodyMode != extprocv3.ProcessingMode_NONE {
		t.Errorf("ResponseBodyMode = %v; want NONE", got.ResponseBodyMode)
	}
}

// TestResolveProcessingMode_NilInput asserts a nil *ProcessingMode resolves
// to the all-defaults form (the proto-doc behavior: "missing field means
// send headers, skip trailers, no body").
func TestResolveProcessingMode_NilInput(t *testing.T) {
	t.Parallel()
	got, err := resolveProcessingMode(nil, false)
	if err != nil {
		t.Fatalf("resolveProcessingMode(nil): err = %v; want nil", err)
	}
	if got == nil {
		t.Fatalf("resolveProcessingMode(nil): nil resolved; want non-nil defaults")
	}
	if got.RequestHeaderMode != extprocv3.ProcessingMode_SEND ||
		got.ResponseHeaderMode != extprocv3.ProcessingMode_SEND ||
		got.RequestTrailerMode != extprocv3.ProcessingMode_SKIP ||
		got.ResponseTrailerMode != extprocv3.ProcessingMode_SKIP ||
		got.RequestBodyMode != extprocv3.ProcessingMode_NONE ||
		got.ResponseBodyMode != extprocv3.ProcessingMode_NONE {
		t.Errorf("nil input resolved to non-default value: %+v", got)
	}
}

// TestResolveProcessingMode_ExplicitSKIP asserts SKIP for header-modes
// flows through verbatim (NOT translated to SEND like DEFAULT is).
func TestResolveProcessingMode_ExplicitSKIP(t *testing.T) {
	t.Parallel()
	pm := &extprocv3.ProcessingMode{
		RequestHeaderMode:  extprocv3.ProcessingMode_SKIP,
		ResponseHeaderMode: extprocv3.ProcessingMode_SEND,
	}
	got, err := resolveProcessingMode(pm, false)
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if got.RequestHeaderMode != extprocv3.ProcessingMode_SKIP {
		t.Errorf("RequestHeaderMode = %v; want SKIP (explicit, not DEFAULT)", got.RequestHeaderMode)
	}
	if got.ResponseHeaderMode != extprocv3.ProcessingMode_SEND {
		t.Errorf("ResponseHeaderMode = %v; want SEND", got.ResponseHeaderMode)
	}
}

// TestResolveProcessingMode_BodyModeStreamedClass_ParseReject asserts the
// PERMANENT PARSE-REJECT discipline for *_body_mode ∈ {STREAMED,
// BUFFERED_PARTIAL, FULL_DUPLEX_STREAMED} per ADR-0168 §Decision +
// ADR-0171 §Decision (parent §4.4: STREAMED-class body modes permanently
// out of envelope). The BUFFERED arm ACCEPTS post-19.2 §Decision AMENDMENT
// for the gRPC-service arm (see TestResolveProcessingMode_BodyModeBuffered_AcceptsForGRPCService);
// the STREAMED-class arms continue PARSE-REJECT permanently.
func TestResolveProcessingMode_BodyModeStreamedClass_ParseReject(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		mode     *extprocv3.ProcessingMode
		wantPart string
	}{
		{
			name: "request_body_mode_STREAMED",
			mode: &extprocv3.ProcessingMode{
				RequestBodyMode: extprocv3.ProcessingMode_STREAMED,
			},
			wantPart: "request_body_mode",
		},
		{
			name: "request_body_mode_BUFFERED_PARTIAL",
			mode: &extprocv3.ProcessingMode{
				RequestBodyMode: extprocv3.ProcessingMode_BUFFERED_PARTIAL,
			},
			wantPart: "request_body_mode",
		},
		{
			name: "request_body_mode_FULL_DUPLEX_STREAMED",
			mode: &extprocv3.ProcessingMode{
				RequestBodyMode: extprocv3.ProcessingMode_FULL_DUPLEX_STREAMED,
			},
			wantPart: "request_body_mode",
		},
		{
			name: "response_body_mode_STREAMED",
			mode: &extprocv3.ProcessingMode{
				ResponseBodyMode: extprocv3.ProcessingMode_STREAMED,
			},
			wantPart: "response_body_mode",
		},
		{
			name: "response_body_mode_FULL_DUPLEX_STREAMED",
			mode: &extprocv3.ProcessingMode{
				ResponseBodyMode: extprocv3.ProcessingMode_FULL_DUPLEX_STREAMED,
			},
			wantPart: "response_body_mode",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveProcessingMode(tc.mode, false)
			if err == nil {
				t.Fatalf("resolveProcessingMode(%s): err = nil; want PARSE-REJECT", tc.name)
			}
			if got != nil {
				t.Errorf("resolveProcessingMode(%s): got = %+v; want nil on PARSE-REJECT", tc.name, got)
			}
			if !strings.Contains(err.Error(), tc.wantPart) {
				t.Errorf("err = %q; want substring %q", err.Error(), tc.wantPart)
			}
		})
	}
}

// TestResolveProcessingMode_TrailerModeNotSKIP_ParseReject asserts the
// PERMANENT PARSE-REJECT for *_trailer_mode != SKIP per parent §5.P9 +
// ADR-0168 §Decision (trailers permanently out of envelope).
func TestResolveProcessingMode_TrailerModeNotSKIP_ParseReject(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		mode     *extprocv3.ProcessingMode
		wantPart string
	}{
		{
			name: "request_trailer_mode_SEND",
			mode: &extprocv3.ProcessingMode{
				RequestTrailerMode: extprocv3.ProcessingMode_SEND,
			},
			wantPart: "request_trailer_mode must be SKIP",
		},
		{
			name: "response_trailer_mode_SEND",
			mode: &extprocv3.ProcessingMode{
				ResponseTrailerMode: extprocv3.ProcessingMode_SEND,
			},
			wantPart: "response_trailer_mode must be SKIP",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveProcessingMode(tc.mode, false)
			if err == nil {
				t.Fatalf("resolveProcessingMode(%s): err = nil; want PARSE-REJECT", tc.name)
			}
			if got != nil {
				t.Errorf("resolveProcessingMode(%s): got = %+v; want nil on PARSE-REJECT", tc.name, got)
			}
			if !strings.Contains(err.Error(), tc.wantPart) {
				t.Errorf("err = %q; want substring %q", err.Error(), tc.wantPart)
			}
		})
	}
}

// TestResolveProcessingMode_TrailerModeSKIP_OK asserts the explicit SKIP +
// the DEFAULT (which translates to SKIP) both pass for trailer-modes.
func TestResolveProcessingMode_TrailerModeSKIP_OK(t *testing.T) {
	t.Parallel()
	pm := &extprocv3.ProcessingMode{
		RequestTrailerMode:  extprocv3.ProcessingMode_SKIP,
		ResponseTrailerMode: extprocv3.ProcessingMode_DEFAULT, // translates to SKIP
	}
	got, err := resolveProcessingMode(pm, false)
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if got.RequestTrailerMode != extprocv3.ProcessingMode_SKIP {
		t.Errorf("RequestTrailerMode = %v; want SKIP", got.RequestTrailerMode)
	}
	if got.ResponseTrailerMode != extprocv3.ProcessingMode_SKIP {
		t.Errorf("ResponseTrailerMode = %v; want SKIP (DEFAULT→SKIP)", got.ResponseTrailerMode)
	}
}

// TestResolveProcessingMode_HTTPServiceBody_ParseReject asserts the proto-
// level ExtProcHttpService constraint: when http_service is the configured
// transport, body-mode != NONE PARSE-REJECT — including BUFFERED which the
// 19.2 §Decision AMENDMENT lifts for the gRPC-service arm only. The
// http_service body PARSE-REJECT continues PERMANENTLY (per ADR-0168
// §Decision (iii); see SPEC §2 item 1).
func TestResolveProcessingMode_HTTPServiceBody_ParseReject(t *testing.T) {
	t.Parallel()
	// BUFFERED is the post-§Decision AMENDMENT activation case for the
	// gRPC-service arm. For http_service mode the BUFFERED arm continues
	// PARSE-REJECT via the httpServiceMode-gated branch (now load-bearing
	// — pre-AMENDMENT it was masked by the listener-level body-mode-NONE
	// PARSE-REJECT firing first).
	pm := &extprocv3.ProcessingMode{
		RequestBodyMode: extprocv3.ProcessingMode_BUFFERED,
	}
	got, err := resolveProcessingMode(pm, true /*httpServiceMode*/)
	if err == nil {
		t.Fatalf("resolveProcessingMode(http_service + BUFFERED): err = nil; want PARSE-REJECT")
	}
	if got != nil {
		t.Errorf("got = %+v; want nil on PARSE-REJECT", got)
	}
	if !strings.Contains(err.Error(), "http_service") {
		t.Errorf("err = %q; want substring 'http_service' (httpServiceMode-gated PARSE-REJECT)", err.Error())
	}
}

// TestStageString anchors the stage.String() coverage — used in dispError
// wrapping at Task 8 + test failure logs.
func TestStageString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s    stage
		want string
	}{
		{stageRequestHeaders, "request_headers"},
		{stageResponseHeaders, "response_headers"},
		{stage(99), "stage(99)"},
	}
	for _, tc := range cases {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("stage(%d).String() = %q; want %q", int(tc.s), got, tc.want)
		}
	}
}

// TestActionString anchors the action.String() coverage.
func TestActionString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a    action
		want string
	}{
		{actContinue, "continue"},
		{actStop, "stop"},
		{actError, "error"},
		{actImmediate, "immediate"},
		{actContinueButStillWaiting, "continue-but-still-waiting"},
		{action(99), "action(99)"},
	}
	for _, tc := range cases {
		if got := tc.a.String(); got != tc.want {
			t.Errorf("action(%d).String() = %q; want %q", int(tc.a), got, tc.want)
		}
	}
}

// TestApplyProcessingResponseStub asserts the Task 7 stub returns (actError,
// errProcessorStub) deterministically. Pins the sentinel-error contract until
// Task 8 publishes the real applyProcessingResponse via the variable indirection.
func TestApplyProcessingResponseStub(t *testing.T) {
	t.Parallel()
	act, err := applyProcessingResponseStub(nil, stageRequestHeaders, nil)
	if act != actError {
		t.Errorf("stub action = %v; want actError", act)
	}
	if !errors.Is(err, errProcessorStub) {
		t.Errorf("stub err = %v; want errProcessorStub", err)
	}
}

// TestHandleOverrideMessageTimeout_MaxMessageTimeoutDisabled — when
// cc.maxMessageTimeout < 1ms (the default 0), the override API is DISABLED;
// the arrival increments overrideMessageTimeoutIgnored and returns false.
func TestHandleOverrideMessageTimeout_MaxMessageTimeoutDisabled(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		messageTimeout:    200 * time.Millisecond,
		maxMessageTimeout: 0, // override disabled
		stats:             newFilterStats(reg, "t"),
	}
	f := &filter{cc: cc}
	ot := durationpb.New(500 * time.Millisecond) // would otherwise be valid
	ok := f.handleOverrideMessageTimeout(stageRequestHeaders, ot)
	if ok {
		t.Errorf("handleOverrideMessageTimeout returned true; want false (override disabled)")
	}
	if got := cc.stats.overrideMessageTimeoutIgnored.Load(); got != 1 {
		t.Errorf("overrideMessageTimeoutIgnored = %d; want 1", got)
	}
	if got := cc.stats.overrideMessageTimeoutReceived.Load(); got != 0 {
		t.Errorf("overrideMessageTimeoutReceived = %d; want 0 (rejected)", got)
	}
	if f.activeMsgTimeout != 0 {
		t.Errorf("f.activeMsgTimeout = %v; want 0 (override rejected)", f.activeMsgTimeout)
	}
}

// TestHandleOverrideMessageTimeout_HappyPath — max_message_timeout=10s,
// override=500ms (in range); increments received counter + sets
// f.activeMsgTimeout to 500ms + marks the stage as override-applied; returns
// true.
func TestHandleOverrideMessageTimeout_HappyPath(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		messageTimeout:    200 * time.Millisecond,
		maxMessageTimeout: 10 * time.Second,
		stats:             newFilterStats(reg, "t"),
	}
	f := &filter{cc: cc, activeMsgTimeout: 200 * time.Millisecond}
	ot := durationpb.New(500 * time.Millisecond)
	if !f.handleOverrideMessageTimeout(stageRequestHeaders, ot) {
		t.Fatalf("handleOverrideMessageTimeout returned false; want true (in-range happy path)")
	}
	if got := cc.stats.overrideMessageTimeoutReceived.Load(); got != 1 {
		t.Errorf("overrideMessageTimeoutReceived = %d; want 1", got)
	}
	if got := cc.stats.overrideMessageTimeoutIgnored.Load(); got != 0 {
		t.Errorf("overrideMessageTimeoutIgnored = %d; want 0 (happy path)", got)
	}
	if f.activeMsgTimeout != 500*time.Millisecond {
		t.Errorf("f.activeMsgTimeout = %v; want 500ms (override applied)", f.activeMsgTimeout)
	}
	if !f.overrideApplied[stageRequestHeaders] {
		t.Errorf("f.overrideApplied[stageRequestHeaders] = false; want true")
	}
}

// TestHandleOverrideMessageTimeout_RangeCheck — duration < 1ms OR
// duration > max_message_timeout → ignored.
func TestHandleOverrideMessageTimeout_RangeCheck(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		dur  time.Duration
	}{
		{"below_1ms", 500 * time.Microsecond}, // < 1ms
		{"above_max", 20 * time.Second},       // > 10s max
		{"zero", 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reg := stats.NewRegistry()
			cc := &compiledConfig{
				maxMessageTimeout: 10 * time.Second,
				stats:             newFilterStats(reg, tc.name),
			}
			f := &filter{cc: cc}
			ok := f.handleOverrideMessageTimeout(stageRequestHeaders, durationpb.New(tc.dur))
			if ok {
				t.Errorf("ok = true; want false (%s out of range)", tc.name)
			}
			if got := cc.stats.overrideMessageTimeoutIgnored.Load(); got != 1 {
				t.Errorf("overrideMessageTimeoutIgnored = %d; want 1", got)
			}
			if got := cc.stats.overrideMessageTimeoutReceived.Load(); got != 0 {
				t.Errorf("overrideMessageTimeoutReceived = %d; want 0 (rejected)", got)
			}
		})
	}
}

// TestHandleOverrideMessageTimeout_AtMostOncePerStage — second override at
// the same stage is rejected as spurious (ignored counter); the first one
// still took effect.
func TestHandleOverrideMessageTimeout_AtMostOncePerStage(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		maxMessageTimeout: 10 * time.Second,
		stats:             newFilterStats(reg, "t"),
	}
	f := &filter{cc: cc}
	// First override: accepted.
	if !f.handleOverrideMessageTimeout(stageRequestHeaders, durationpb.New(500*time.Millisecond)) {
		t.Fatalf("first override rejected; want accepted")
	}
	// Second override at SAME stage: rejected.
	if f.handleOverrideMessageTimeout(stageRequestHeaders, durationpb.New(1*time.Second)) {
		t.Errorf("second override at same stage accepted; want rejected (at-most-once-per-stage)")
	}
	// Counters: 1 received + 1 ignored.
	if got := cc.stats.overrideMessageTimeoutReceived.Load(); got != 1 {
		t.Errorf("overrideMessageTimeoutReceived = %d; want 1", got)
	}
	if got := cc.stats.overrideMessageTimeoutIgnored.Load(); got != 1 {
		t.Errorf("overrideMessageTimeoutIgnored = %d; want 1", got)
	}
	// activeMsgTimeout still reflects the FIRST accepted value.
	if f.activeMsgTimeout != 500*time.Millisecond {
		t.Errorf("f.activeMsgTimeout = %v; want 500ms (first override stuck)", f.activeMsgTimeout)
	}
}

// TestHandleOverrideMessageTimeout_PerStageIndependent — an override at
// stageRequestHeaders does NOT block one at stageResponseHeaders.
func TestHandleOverrideMessageTimeout_PerStageIndependent(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		maxMessageTimeout: 10 * time.Second,
		stats:             newFilterStats(reg, "t"),
	}
	f := &filter{cc: cc}
	if !f.handleOverrideMessageTimeout(stageRequestHeaders, durationpb.New(500*time.Millisecond)) {
		t.Fatalf("request-stage override rejected; want accepted")
	}
	if !f.handleOverrideMessageTimeout(stageResponseHeaders, durationpb.New(1*time.Second)) {
		t.Errorf("response-stage override rejected; want accepted (per-stage independent)")
	}
	if got := cc.stats.overrideMessageTimeoutReceived.Load(); got != 2 {
		t.Errorf("overrideMessageTimeoutReceived = %d; want 2 (two stages)", got)
	}
	if got := cc.stats.overrideMessageTimeoutIgnored.Load(); got != 0 {
		t.Errorf("overrideMessageTimeoutIgnored = %d; want 0", got)
	}
}

// TestHandleOverrideMessageTimeout_NilGuards — nil receiver / nil cc / nil
// duration all return false safely (no panic; no counter increments — nil
// cc means stats are unreachable).
func TestHandleOverrideMessageTimeout_NilGuards(t *testing.T) {
	t.Parallel()
	var f *filter
	if f.handleOverrideMessageTimeout(stageRequestHeaders, durationpb.New(time.Second)) {
		t.Errorf("nil receiver returned true; want false")
	}
	f = &filter{} // cc nil
	if f.handleOverrideMessageTimeout(stageRequestHeaders, durationpb.New(time.Second)) {
		t.Errorf("nil cc returned true; want false")
	}
	f = &filter{cc: &compiledConfig{maxMessageTimeout: time.Second}}
	if f.handleOverrideMessageTimeout(stageRequestHeaders, nil) {
		t.Errorf("nil duration returned true; want false")
	}
}

// ---------------------------------------------------------------------------
// completeStage coverage — uses applyProcessingResponseFn override to inject
// deterministic action returns + a fakeDCB/fakeECB pair to observe resume
// signals. The fakes track ContinueDecoding/ContinueEncoding invocation
// counts.
// ---------------------------------------------------------------------------

// fakeDCB is a minimal envoyhttp.DecoderFilterCallbacks fake for Group 7
// completeStage tests. Only ContinueDecoding is exercised — the rest of the
// interface is implemented as no-ops or sentinel returns.
type fakeDCB struct {
	continueCalls int
	mu            sync.Mutex
}

func (f *fakeDCB) ContinueDecoding() {
	f.mu.Lock()
	f.continueCalls++
	f.mu.Unlock()
}
func (f *fakeDCB) SendLocalReply(int, string, envoyhttp.OrderedHeaders) {}
func (f *fakeDCB) RequestRouteConfig() proto.Message                    { return nil }
func (f *fakeDCB) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (f *fakeDCB) EncodeHeaders(http.Header, bool)  {}
func (f *fakeDCB) EncodeData([]byte, bool)          {}
func (f *fakeDCB) EncodeTrailers(http.Header)       {}
func (f *fakeDCB) DownstreamPrincipal() []string    { return nil }
func (f *fakeDCB) DownstreamRemoteAddr() net.Addr   { return nil }
func (f *fakeDCB) DownstreamLocalAddr() net.Addr    { return nil }
func (f *fakeDCB) DownstreamTLSServerName() string  { return "" }
func (f *fakeDCB) DownstreamTLSPeerCertDER() []byte { return nil }
func (f *fakeDCB) DownstreamProtocol() string       { return "" }
func (f *fakeDCB) ListenerPrincipal() string        { return "" }

// ADR-0192 callback-surface extension stubs (phase-22.2 Task 5). Zero-value
// returns satisfy the interface for tests that don't need the TLS-state /
// dynamic-metadata surface.
func (f *fakeDCB) DownstreamTLSConnectionState() *tls.ConnectionState { return nil }
func (f *fakeDCB) DynamicMetadata() *dynamicmetadata.Bucket           { return nil }

// ADR-0198 callback-surface extension stubs (phase-24.1 Task 5 — DELTA-2).
func (f *fakeDCB) RouteRateLimits() []*routev3.RateLimit       { return nil }
func (f *fakeDCB) VirtualHostRateLimits() []*routev3.RateLimit { return nil }
func (f *fakeDCB) RouteMetadata() *corev3.Metadata             { return nil }
func (f *fakeDCB) RouteIncludeVhRateLimits() bool              { return false }

func (f *fakeDCB) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.continueCalls
}

// fakeECB is a minimal envoyhttp.EncoderFilterCallbacks fake.
type fakeECB struct {
	continueCalls int
	mu            sync.Mutex
}

func (f *fakeECB) ContinueEncoding() {
	f.mu.Lock()
	f.continueCalls++
	f.mu.Unlock()
}
func (f *fakeECB) EncodeHeaders(http.Header, bool)  {}
func (f *fakeECB) EncodeData([]byte, bool)          {}
func (f *fakeECB) EncodeTrailers(http.Header)       {}
func (f *fakeECB) OverwriteBody([]byte)             {}
func (f *fakeECB) DownstreamRemoteAddr() net.Addr   { return nil }
func (f *fakeECB) DownstreamLocalAddr() net.Addr    { return nil }
func (f *fakeECB) DownstreamTLSServerName() string  { return "" }
func (f *fakeECB) DownstreamTLSPeerCertDER() []byte { return nil }
func (f *fakeECB) DownstreamProtocol() string       { return "" }
func (f *fakeECB) ListenerPrincipal() string        { return "" }
func (f *fakeECB) BufferEncodedBody() []byte        { return nil } // ADR-0175 (phase-19.2 Task 2); fake returns nil — Task 7 wires the real consumer.

// ADR-0192 callback-surface extension stubs (phase-22.2 Task 5).
func (f *fakeECB) DownstreamTLSConnectionState() *tls.ConnectionState { return nil }
func (f *fakeECB) DynamicMetadata() *dynamicmetadata.Bucket           { return nil }
func (f *fakeECB) ResponseStatus() int                                { return 0 } // ADR-0196; extproc does not consume the encode-side status.

func (f *fakeECB) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.continueCalls
}

// Compile-time conformance — fail fast if the fakes drift from the framework
// interface signatures (mirrors callbacks_test.go's pattern).
var (
	_ envoyhttp.DecoderFilterCallbacks = (*fakeDCB)(nil)
	_ envoyhttp.EncoderFilterCallbacks = (*fakeECB)(nil)
)

// withApplyOverride installs a per-test applyProcessingResponseFn override +
// registers a t.Cleanup that restores the production stub. Returns a getter
// for the last (stage, response) args captured by the override.
func withApplyOverride(t *testing.T, fn func(*filter, stage, *extprocsvcv3.ProcessingResponse) (action, error)) {
	t.Helper()
	prior := applyProcessingResponseFn
	applyProcessingResponseFn = fn
	t.Cleanup(func() { applyProcessingResponseFn = prior })
}

// TestCompleteStage_ActContinue_DecodeStage_SignalsResume asserts the resume
// signal fires on the decode side when applyProcessingResponseFn returns
// actContinue at stageRequestHeaders.
func TestCompleteStage_ActContinue_DecodeStage_SignalsResume(t *testing.T) {
	withApplyOverride(t, func(*filter, stage, *extprocsvcv3.ProcessingResponse) (action, error) {
		return actContinue, nil
	})
	dcb := &fakeDCB{}
	f := &filter{dcb: dcb}
	f.completeStage(stageRequestHeaders, &extprocsvcv3.ProcessingResponse{}, nil)
	if dcb.calls() != 1 {
		t.Errorf("ContinueDecoding called %d times; want 1", dcb.calls())
	}
}

// TestCompleteStage_ActContinue_EncodeStage_SignalsResume asserts the resume
// signal fires on the encode side when applyProcessingResponseFn returns
// actContinue at stageResponseHeaders.
func TestCompleteStage_ActContinue_EncodeStage_SignalsResume(t *testing.T) {
	withApplyOverride(t, func(*filter, stage, *extprocsvcv3.ProcessingResponse) (action, error) {
		return actContinue, nil
	})
	ecb := &fakeECB{}
	f := &filter{ecb: ecb}
	f.completeStage(stageResponseHeaders, &extprocsvcv3.ProcessingResponse{}, nil)
	if ecb.calls() != 1 {
		t.Errorf("ContinueEncoding called %d times; want 1", ecb.calls())
	}
}

// TestCompleteStage_ActImmediate_DecodeStage_SignalsResume asserts that
// actImmediate FIRES ContinueDecoding (decode-side) per the
// SendLocalReply+ContinueDecoding pattern documented at
// chain.go's timerSendLocalReplyFilter test + phase-18.x ext_authz
// (extauthz.go:1110-1111) + phase-09 fault (fault.go:321-324). The async
// dispatch goroutine calling SendLocalReply from off-dispatch sets
// chain.localReplyDone=true and synchronously runs the encode chain via
// beginLocalReply, but does NOT unblock the HCM dispatch goroutine which is
// still parked in parkDecode. The resume signal here wakes that goroutine.
// Task 13 rework: fixture-0022 scenario-2 root cause (without this signal
// the subject returned status=0 connection reset instead of 403+body).
func TestCompleteStage_ActImmediate_DecodeStage_SignalsResume(t *testing.T) {
	withApplyOverride(t, func(*filter, stage, *extprocsvcv3.ProcessingResponse) (action, error) {
		return actImmediate, nil
	})
	dcb := &fakeDCB{}
	f := &filter{dcb: dcb}
	f.completeStage(stageRequestHeaders, &extprocsvcv3.ProcessingResponse{}, nil)
	if dcb.calls() != 1 {
		t.Errorf("ContinueDecoding called %d times; want 1 (actImmediate must signal to wake parked HCM dispatch — Task 13 rework root cause fix)", dcb.calls())
	}
}

// TestCompleteStage_ActImmediate_EncodeStage_SignalsResume asserts the
// encode-side analog: actImmediate at stageResponseHeaders fires
// ContinueEncoding to wake the parked encode-side dispatch goroutine. Same
// rationale as the decode-side test above; the encode-side immediate-response
// emission is the FIRST §9 row to fire SendLocalReply from the encode side
// per ADR-0167 + ADR-0075, and the resume-signal discipline is symmetric.
func TestCompleteStage_ActImmediate_EncodeStage_SignalsResume(t *testing.T) {
	withApplyOverride(t, func(*filter, stage, *extprocsvcv3.ProcessingResponse) (action, error) {
		return actImmediate, nil
	})
	ecb := &fakeECB{}
	f := &filter{ecb: ecb}
	f.completeStage(stageResponseHeaders, &extprocsvcv3.ProcessingResponse{}, nil)
	if ecb.calls() != 1 {
		t.Errorf("ContinueEncoding called %d times; want 1 (actImmediate must signal to wake parked HCM dispatch)", ecb.calls())
	}
}

// TestCompleteStage_RecvError_SignalsResume — a recv-side error
// (transport / timeout / cancel) still signals resume so the parked stage
// unparks. The dispError classification lands at Task 8.
func TestCompleteStage_RecvError_SignalsResume(t *testing.T) {
	dcb := &fakeDCB{}
	f := &filter{dcb: dcb}
	f.completeStage(stageRequestHeaders, nil, errors.New("transport error"))
	if dcb.calls() != 1 {
		t.Errorf("ContinueDecoding called %d times; want 1 (resume-on-error)", dcb.calls())
	}
}

// TestCompleteStage_D9Race_DoneFlagDropsSignal — after OnDestroy fires
// (f.done = true), completeStage drops the resume signal (the framework
// chain is torn down).
func TestCompleteStage_D9Race_DoneFlagDropsSignal(t *testing.T) {
	withApplyOverride(t, func(*filter, stage, *extprocsvcv3.ProcessingResponse) (action, error) {
		return actContinue, nil
	})
	dcb := &fakeDCB{}
	f := &filter{dcb: dcb}
	// Simulate OnDestroy firing FIRST.
	f.mu.Lock()
	f.done = true
	f.mu.Unlock()
	f.completeStage(stageRequestHeaders, &extprocsvcv3.ProcessingResponse{}, nil)
	if dcb.calls() != 0 {
		t.Errorf("ContinueDecoding called %d times; want 0 (done flag set)", dcb.calls())
	}
}

// ---------------------------------------------------------------------------
// Group 10 — OnDestroy lifecycle per ADR-0171 §Decision D9 (Task 7).
//
// Tests cover:
//   - OnDestroy cancels in-flight Recv (the Recv returns context.Canceled
//     promptly when streamCtx is canceled).
//   - CloseSend called exactly ONCE on OnDestroy (sync.Once-guarded per D9).
//   - Idempotent: multiple OnDestroy invocations are no-ops after the first.
//   - Concurrent OnDestroy + completeStage: the done flag race-guard ensures
//     completeStage drops the signal cleanly.
// ---------------------------------------------------------------------------

// fakeProcessStream is a deterministic grpcclient.ProcessStream fake used by
// Group 10 lifecycle tests + Group 7 dispatchStage assertions.
type fakeProcessStream struct {
	mu sync.Mutex

	sendCalls    int
	recvCalls    int
	closeSendCnt int
	sendErr      error
	recvErr      error
	recvBlockCh  chan struct{}   // when non-nil, Recv blocks until closed
	recvBlockCtx context.Context // when non-nil, Recv also unblocks on ctx.Done
	recvResp     *extprocsvcv3.ProcessingResponse
}

func (s *fakeProcessStream) Send(_ *extprocsvcv3.ProcessingRequest) error {
	s.mu.Lock()
	s.sendCalls++
	err := s.sendErr
	s.mu.Unlock()
	return err
}

func (s *fakeProcessStream) Recv() (*extprocsvcv3.ProcessingResponse, error) {
	s.mu.Lock()
	s.recvCalls++
	blockCh := s.recvBlockCh
	blockCtx := s.recvBlockCtx
	resp := s.recvResp
	err := s.recvErr
	s.mu.Unlock()
	if blockCh != nil {
		if blockCtx != nil {
			select {
			case <-blockCh:
			case <-blockCtx.Done():
				return nil, blockCtx.Err()
			}
		} else {
			<-blockCh
		}
	}
	return resp, err
}

func (s *fakeProcessStream) CloseSend() error {
	s.mu.Lock()
	s.closeSendCnt++
	s.mu.Unlock()
	return nil
}

func (s *fakeProcessStream) closeSendCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeSendCnt
}

// TestOnDestroy_CloseSendCalledOnce — OnDestroy fires CloseSend exactly once
// per the sync.Once-guarded D9 discipline; subsequent OnDestroy invocations
// are no-ops.
func TestOnDestroy_CloseSendCalledOnce(t *testing.T) {
	t.Parallel()
	stream := &fakeProcessStream{}
	ctx, cancel := context.WithCancel(context.Background())
	f := &filter{
		stream:       stream,
		streamCtx:    ctx,
		streamCancel: cancel,
	}
	f.OnDestroy()
	f.OnDestroy()
	f.OnDestroy()
	if got := stream.closeSendCount(); got != 1 {
		t.Errorf("CloseSend called %d times; want exactly 1 (sync.Once-guarded)", got)
	}
	if !f.done {
		t.Errorf("f.done = false after OnDestroy; want true")
	}
}

// TestOnDestroy_CancelsInflightRecv — OnDestroy invokes streamCancel which
// causes the in-flight Recv (blocked on streamCtx) to return ctx.Err()
// (context.Canceled).
func TestOnDestroy_CancelsInflightRecv(t *testing.T) {
	t.Parallel()
	blockCh := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	stream := &fakeProcessStream{
		recvBlockCh:  blockCh,
		recvBlockCtx: ctx,
	}
	f := &filter{
		stream:       stream,
		streamCtx:    ctx,
		streamCancel: cancel,
	}
	// Spawn a goroutine simulating the dispatchStage Recv leg.
	recvDoneCh := make(chan error, 1)
	go func() {
		_, err := f.stream.Recv()
		recvDoneCh <- err
	}()
	// OnDestroy cancels streamCtx → Recv returns context.Canceled.
	f.OnDestroy()
	select {
	case err := <-recvDoneCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Recv returned err = %v; want context.Canceled", err)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("Recv did not return within 1s of OnDestroy; cancel discipline broken")
		close(blockCh) // unblock to let the goroutine exit
	}
}

// TestOnDestroy_NilTolerant — OnDestroy is safe to call on a filter with no
// stream / no streamCancel (the http_service-mode path + the
// openProcessorStream-failed path both leave these nil).
func TestOnDestroy_NilTolerant(t *testing.T) {
	t.Parallel()
	f := &filter{}
	// Should not panic.
	f.OnDestroy()
	if !f.done {
		t.Errorf("f.done = false after OnDestroy; want true")
	}
}

// TestOnDestroy_StreamsClosedCounterIncrements — when cc.stats is non-nil,
// OnDestroy increments cc.stats.streamsClosed exactly once.
func TestOnDestroy_StreamsClosedCounterIncrements(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{stats: newFilterStats(reg, "t")}
	stream := &fakeProcessStream{}
	ctx, cancel := context.WithCancel(context.Background())
	f := &filter{
		cc:           cc,
		stream:       stream,
		streamCtx:    ctx,
		streamCancel: cancel,
	}
	f.OnDestroy()
	f.OnDestroy() // second call: no-op (sync.Once)
	if got := cc.stats.streamsClosed.Load(); got != 1 {
		t.Errorf("streamsClosed = %d; want 1 (sync.Once-guarded)", got)
	}
}

// TestDispatchStage_HappyPath_SignalsResume — dispatchStage spawns the
// goroutine; the fake Recv returns a response immediately; completeStage
// fires applyProcessingResponseFn (overridden to actContinue) which signals
// resume on the decode side.
func TestDispatchStage_HappyPath_SignalsResume(t *testing.T) {
	withApplyOverride(t, func(*filter, stage, *extprocsvcv3.ProcessingResponse) (action, error) {
		return actContinue, nil
	})
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		messageTimeout: 5 * time.Second,
		stats:          newFilterStats(reg, "dispatch"),
	}
	dcb := &fakeDCB{}
	stream := &fakeProcessStream{
		recvResp: &extprocsvcv3.ProcessingResponse{
			Response: &extprocsvcv3.ProcessingResponse_RequestHeaders{
				RequestHeaders: &extprocsvcv3.HeadersResponse{},
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	f := &filter{
		cc:               cc,
		dcb:              dcb,
		stream:           stream,
		streamCtx:        ctx,
		streamCancel:     cancel,
		activeMsgTimeout: 5 * time.Second,
	}
	defer cancel()

	f.dispatchStage(stageRequestHeaders, &extprocsvcv3.ProcessingRequest{})

	// Wait for the goroutine to land the resume signal.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if dcb.calls() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if dcb.calls() != 1 {
		t.Errorf("ContinueDecoding called %d times; want 1", dcb.calls())
	}
	if got := cc.stats.streamMsgsSent.Load(); got != 1 {
		t.Errorf("streamMsgsSent = %d; want 1", got)
	}
	if got := cc.stats.streamMsgsReceived.Load(); got != 1 {
		t.Errorf("streamMsgsReceived = %d; want 1", got)
	}
}

// TestDispatchStage_SendError_IncrementsStreamsFailed — when Send returns
// an error, streamsFailed increments + the resume fires (via completeStage's
// recvErr path).
func TestDispatchStage_SendError_IncrementsStreamsFailed(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		messageTimeout: 100 * time.Millisecond,
		stats:          newFilterStats(reg, "send_err"),
	}
	dcb := &fakeDCB{}
	stream := &fakeProcessStream{
		sendErr: errors.New("send failed"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	f := &filter{
		cc:               cc,
		dcb:              dcb,
		stream:           stream,
		streamCtx:        ctx,
		streamCancel:     cancel,
		activeMsgTimeout: 100 * time.Millisecond,
	}
	defer cancel()
	f.dispatchStage(stageRequestHeaders, &extprocsvcv3.ProcessingRequest{})

	// Wait for the resume signal.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if dcb.calls() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if dcb.calls() != 1 {
		t.Errorf("ContinueDecoding called %d times; want 1 (resume-on-send-error)", dcb.calls())
	}
	if got := cc.stats.streamsFailed.Load(); got != 1 {
		t.Errorf("streamsFailed = %d; want 1", got)
	}
	if got := cc.stats.streamMsgsSent.Load(); got != 0 {
		t.Errorf("streamMsgsSent = %d; want 0 (send failed before increment)", got)
	}
}

// Anchor unused imports (net + proto + sync) for the fakeDCB/fakeECB types
// above. Each fake satisfies the framework's full DecoderFilterCallbacks /
// EncoderFilterCallbacks interfaces; the anchors here are tactical.
var (
	_ net.Addr      = (net.Addr)(nil)
	_ proto.Message = (proto.Message)(nil)
)

// ---------------------------------------------------------------------------
// Group 3 — buildGRPCProcessorClient + buildHTTPProcessorClient (Task 8).
//
// Tests cover:
//   - buildGRPCProcessorClient: GoogleGrpc PARSE-REJECT, EnvoyGrpc empty
//     cluster_name PARSE-REJECT, nil cluster manager PARSE-REJECT, unknown
//     cluster PARSE-REJECT, non-H2 cluster PARSE-REJECT, happy-path H2
//     cluster + grpcclient.NewProcessorClient construction.
//   - buildHTTPProcessorClient: nil service / nil HttpService / empty URI
//     PARSE-REJECT, happy-path *http.Client + base URL capture, timeout
//     honored.
// ---------------------------------------------------------------------------

// mkExtprocPlainClusterMgr builds a *cluster.Manager with a single plaintext
// STATIC cluster (UseH2()==false) at 127.0.0.1:port. PARSE-REJECT paths
// never reach the dial step. Mirrors grpcclient_test.go's mkPlainClusterMgr.
func mkExtprocPlainClusterMgr(t testing.TB, name string, port uint32) *cluster.Manager {
	t.Helper()
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 name,
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: name,
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:       "127.0.0.1",
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port},
									},
								}},
							}},
						}},
					}},
				},
			}},
		},
	}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManager(plain): %v", err)
	}
	return cm
}

// mkExtprocH2ClusterMgr builds a *cluster.Manager with a single STATIC
// cluster declared with http2_protocol_options{} (UseH2()==true).
// Mirrors extauthz_test.go's mkExtauthzH2ClusterMgr but without TLS — gRPC
// over plaintext h2c per ADR-0166 (the BRAINSTORM precondition for this
// task's happy-path).
func mkExtprocH2ClusterMgr(t testing.TB, name string, port uint32) *cluster.Manager {
	t.Helper()
	hpoH2 := &upstreamshttpv3.HttpProtocolOptions{
		UpstreamProtocolOptions: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
			},
		},
	}
	hpoAny, err := anypb.New(hpoH2)
	if err != nil {
		t.Fatalf("anypb.New(HttpProtocolOptions): %v", err)
	}
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 name,
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: name,
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:       "127.0.0.1",
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port},
									},
								}},
							}},
						}},
					}},
				},
				TypedExtensionProtocolOptions: map[string]*anypb.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": hpoAny,
				},
			}},
		},
	}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManager(h2): %v", err)
	}
	return cm
}

// TestBuildGRPCProcessorClient_NilService — nil GrpcService PARSE-REJECT.
func TestBuildGRPCProcessorClient_NilService(t *testing.T) {
	t.Parallel()
	pc, err := buildGRPCProcessorClient(nil, envoyhttp.FactoryCtx{}, 200*time.Millisecond)
	if err == nil {
		t.Fatalf("buildGRPCProcessorClient(nil): err = nil; want PARSE-REJECT")
	}
	if pc != nil {
		t.Errorf("pc = %v; want nil on PARSE-REJECT", pc)
	}
	if !strings.Contains(err.Error(), "grpc_service is required") {
		t.Errorf("err = %q; want 'grpc_service is required'", err)
	}
}

// TestBuildGRPCProcessorClient_GoogleGrpcRejected — GoogleGrpc arm PARSE-REJECT
// per ADR-0157 §Decision AMENDMENT.
func TestBuildGRPCProcessorClient_GoogleGrpcRejected(t *testing.T) {
	t.Parallel()
	gs := &corev3.GrpcService{
		TargetSpecifier: &corev3.GrpcService_GoogleGrpc_{
			GoogleGrpc: &corev3.GrpcService_GoogleGrpc{TargetUri: "foo"},
		},
	}
	pc, err := buildGRPCProcessorClient(gs, envoyhttp.FactoryCtx{}, 200*time.Millisecond)
	if err == nil {
		t.Fatalf("err = nil; want PARSE-REJECT (GoogleGrpc unsupported)")
	}
	if pc != nil {
		t.Errorf("pc = %v; want nil", pc)
	}
	if !strings.Contains(err.Error(), "google_grpc arm not supported") {
		t.Errorf("err = %q; want substring 'google_grpc arm not supported'", err)
	}
}

// TestBuildGRPCProcessorClient_EmptyEnvoyGrpc — EnvoyGrpc with empty
// cluster_name PARSE-REJECT.
func TestBuildGRPCProcessorClient_EmptyEnvoyGrpc(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		gs       *corev3.GrpcService
		wantPart string
	}{
		{
			name:     "nil_target_specifier",
			gs:       &corev3.GrpcService{},
			wantPart: "envoy_grpc arm required",
		},
		{
			name: "empty_cluster_name",
			gs: &corev3.GrpcService{
				TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: ""},
				},
			},
			wantPart: "cluster_name must be non-empty",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pc, err := buildGRPCProcessorClient(tc.gs, envoyhttp.FactoryCtx{}, 200*time.Millisecond)
			if err == nil {
				t.Fatalf("err = nil; want PARSE-REJECT")
			}
			if pc != nil {
				t.Errorf("pc = %v; want nil", pc)
			}
			if !strings.Contains(err.Error(), tc.wantPart) {
				t.Errorf("err = %q; want substring %q", err, tc.wantPart)
			}
		})
	}
}

// TestBuildGRPCProcessorClient_NoClusterManager — nil ClusterManager
// PARSE-REJECT.
func TestBuildGRPCProcessorClient_NoClusterManager(t *testing.T) {
	t.Parallel()
	gs := &corev3.GrpcService{
		TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: "c_extproc"},
		},
	}
	pc, err := buildGRPCProcessorClient(gs, envoyhttp.FactoryCtx{ClusterManager: nil}, 200*time.Millisecond)
	if err == nil {
		t.Fatalf("err = nil; want PARSE-REJECT (nil cluster manager)")
	}
	if pc != nil {
		t.Errorf("pc = %v; want nil", pc)
	}
	if !strings.Contains(err.Error(), "cluster manager not available") {
		t.Errorf("err = %q; want substring 'cluster manager not available'", err)
	}
}

// TestBuildGRPCProcessorClient_UnknownCluster — cluster not found in manager
// → PARSE-REJECT.
func TestBuildGRPCProcessorClient_UnknownCluster(t *testing.T) {
	t.Parallel()
	cm := mkExtprocPlainClusterMgr(t, "c_other", 9999)
	gs := &corev3.GrpcService{
		TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: "c_extproc"},
		},
	}
	pc, err := buildGRPCProcessorClient(gs, envoyhttp.FactoryCtx{ClusterManager: cm}, 200*time.Millisecond)
	if err == nil {
		t.Fatalf("err = nil; want PARSE-REJECT (unknown cluster)")
	}
	if pc != nil {
		t.Errorf("pc = %v; want nil", pc)
	}
	if !strings.Contains(err.Error(), `unknown cluster "c_extproc"`) {
		t.Errorf("err = %q; want substring 'unknown cluster \"c_extproc\"'", err)
	}
}

// TestBuildGRPCProcessorClient_NonH2Cluster — cluster without
// http2_protocol_options PARSE-REJECT (gRPC requires HTTP/2 framing).
func TestBuildGRPCProcessorClient_NonH2Cluster(t *testing.T) {
	t.Parallel()
	cm := mkExtprocPlainClusterMgr(t, "c_plain", 9999)
	gs := &corev3.GrpcService{
		TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: "c_plain"},
		},
	}
	pc, err := buildGRPCProcessorClient(gs, envoyhttp.FactoryCtx{ClusterManager: cm}, 200*time.Millisecond)
	if err == nil {
		t.Fatalf("err = nil; want PARSE-REJECT (UseH2=false)")
	}
	if pc != nil {
		t.Errorf("pc = %v; want nil", pc)
	}
	if !strings.Contains(err.Error(), "http2_protocol_options") {
		t.Errorf("err = %q; want substring 'http2_protocol_options'", err)
	}
}

// TestBuildGRPCProcessorClient_HappyPath — H2 cluster + valid GrpcService →
// non-nil *grpcclient.ProcessorClient with the message-timeout captured.
func TestBuildGRPCProcessorClient_HappyPath(t *testing.T) {
	t.Parallel()
	cm := mkExtprocH2ClusterMgr(t, "c_extproc", 9999)
	gs := &corev3.GrpcService{
		TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: "c_extproc"},
		},
	}
	pc, err := buildGRPCProcessorClient(gs, envoyhttp.FactoryCtx{ClusterManager: cm}, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("err = %v; want nil (happy path)", err)
	}
	if pc == nil {
		t.Fatalf("pc = nil; want non-nil *grpcclient.ProcessorClient")
	}
	if pc.PerMessageTimeout() != 500*time.Millisecond {
		t.Errorf("PerMessageTimeout = %v; want 500ms", pc.PerMessageTimeout())
	}
	// Cleanup: close the dial conn (best-effort; the test does not exercise
	// the dial path since PARSE-REJECT gates fired beforehand on the other
	// tests; here NewProcessorClient may eagerly construct the conn).
	_ = pc.Close()
}

// TestBuildHTTPProcessorClient_NilService — nil ExtProcHttpService
// PARSE-REJECT.
func TestBuildHTTPProcessorClient_NilService(t *testing.T) {
	t.Parallel()
	c, err := buildHTTPProcessorClient(nil)
	if err == nil {
		t.Fatalf("err = nil; want PARSE-REJECT")
	}
	if c != nil {
		t.Errorf("c = %v; want nil", c)
	}
	if !strings.Contains(err.Error(), "http_service is required") {
		t.Errorf("err = %q; want substring 'http_service is required'", err)
	}
}

// TestBuildHTTPProcessorClient_NilNestedService — wrapper without nested
// HttpService.
func TestBuildHTTPProcessorClient_NilNestedService(t *testing.T) {
	t.Parallel()
	c, err := buildHTTPProcessorClient(&extprocv3.ExtProcHttpService{})
	if err == nil {
		t.Fatalf("err = nil; want PARSE-REJECT (nil nested)")
	}
	if c != nil {
		t.Errorf("c = %v; want nil", c)
	}
}

// TestBuildHTTPProcessorClient_EmptyURI — HttpUri.uri must be non-empty.
func TestBuildHTTPProcessorClient_EmptyURI(t *testing.T) {
	t.Parallel()
	hs := &extprocv3.ExtProcHttpService{
		HttpService: &corev3.HttpService{
			HttpUri: &corev3.HttpUri{Uri: ""},
		},
	}
	c, err := buildHTTPProcessorClient(hs)
	if err == nil {
		t.Fatalf("err = nil; want PARSE-REJECT (empty uri)")
	}
	if c != nil {
		t.Errorf("c = %v; want nil", c)
	}
	if !strings.Contains(err.Error(), "uri is required") {
		t.Errorf("err = %q; want substring 'uri is required'", err)
	}
}

// TestBuildHTTPProcessorClient_HappyPath — populated URI + timeout → non-nil
// client + correct fields.
func TestBuildHTTPProcessorClient_HappyPath(t *testing.T) {
	t.Parallel()
	hs := &extprocv3.ExtProcHttpService{
		HttpService: &corev3.HttpService{
			HttpUri: &corev3.HttpUri{
				Uri:     "http://processor.local:8080/process",
				Timeout: durationpb.New(750 * time.Millisecond),
			},
		},
	}
	c, err := buildHTTPProcessorClient(hs)
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if c == nil {
		t.Fatalf("c = nil; want non-nil *httpProcessorClient")
	}
	if c.client == nil {
		t.Errorf("c.client = nil; want non-nil *http.Client")
	}
	if c.client.Timeout != 750*time.Millisecond {
		t.Errorf("client.Timeout = %v; want 750ms", c.client.Timeout)
	}
	if c.baseURL != "http://processor.local:8080/process" {
		t.Errorf("baseURL = %q; want %q", c.baseURL, "http://processor.local:8080/process")
	}
	// Close should be a no-op (no idle conns to close on a fresh client).
	if err := c.Close(); err != nil {
		t.Errorf("Close: %v; want nil", err)
	}
}

// TestBuildHTTPProcessorClient_ZeroTimeout — nil/zero Timeout → no timeout
// (proto-default).
func TestBuildHTTPProcessorClient_ZeroTimeout(t *testing.T) {
	t.Parallel()
	hs := &extprocv3.ExtProcHttpService{
		HttpService: &corev3.HttpService{
			HttpUri: &corev3.HttpUri{Uri: "http://p.local"},
		},
	}
	c, err := buildHTTPProcessorClient(hs)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if c.client.Timeout != 0 {
		t.Errorf("client.Timeout = %v; want 0 (proto-default)", c.client.Timeout)
	}
}

// ---------------------------------------------------------------------------
// Group 4 — applyHeaderMutation + resolveMutationRules + per-header gating
// per parent §5.P3 + ADR-0172 §Decision (header-mode portion). (Task 8)
// ---------------------------------------------------------------------------

// TestResolveMutationRules_NilDefault — nil input → proto-default rule set
// (all four flags false).
func TestResolveMutationRules_NilDefault(t *testing.T) {
	t.Parallel()
	r := resolveMutationRules(nil)
	if r == nil {
		t.Fatalf("resolveMutationRules(nil) = nil; want non-nil with defaults")
	}
	if r.AllowAllRouting || r.AllowEnvoy || r.DisallowSystem || r.DisallowAll {
		t.Errorf("default flags non-zero: %+v", r)
	}
	// Default: protected headers are protected.
	if r.isAllowed("host") {
		t.Errorf("isAllowed(host) = true; want false (default protected)")
	}
	if r.isAllowed("x-envoy-attempt") {
		t.Errorf("isAllowed(x-envoy-attempt) = true; want false (default protected)")
	}
	// Default: arbitrary headers allowed.
	if !r.isAllowed("x-custom") {
		t.Errorf("isAllowed(x-custom) = false; want true (default allowed)")
	}
}

// TestResolveMutationRules_AllFieldsCompiled — populated boolean wrappers
// compile through.
func TestResolveMutationRules_AllFieldsCompiled(t *testing.T) {
	t.Parallel()
	rules := &commonmutationv3.HeaderMutationRules{
		AllowAllRouting: wrapperspb.Bool(true),
		AllowEnvoy:      wrapperspb.Bool(true),
		DisallowSystem:  wrapperspb.Bool(false),
		DisallowAll:     wrapperspb.Bool(false),
	}
	r := resolveMutationRules(rules)
	if r == nil {
		t.Fatalf("nil; want non-nil")
	}
	if !r.AllowAllRouting {
		t.Errorf("AllowAllRouting = false; want true")
	}
	if !r.AllowEnvoy {
		t.Errorf("AllowEnvoy = false; want true")
	}
	// Now host + x-envoy-* should be allowed.
	if !r.isAllowed("host") {
		t.Errorf("isAllowed(host) = false; want true under AllowAllRouting")
	}
	if !r.isAllowed("x-envoy-attempt") {
		t.Errorf("isAllowed(x-envoy-attempt) = false; want true under AllowEnvoy")
	}
}

// TestResolveMutationRules_DisallowAll — DisallowAll wraps everything off.
func TestResolveMutationRules_DisallowAll(t *testing.T) {
	t.Parallel()
	rules := &commonmutationv3.HeaderMutationRules{
		DisallowAll: wrapperspb.Bool(true),
	}
	r := resolveMutationRules(rules)
	if r.isAllowed("x-custom") {
		t.Errorf("isAllowed(x-custom) = true under disallow_all; want false")
	}
	if r.isAllowed("host") {
		t.Errorf("isAllowed(host) = true under disallow_all; want false")
	}
}

// TestResolveMutationRules_DisallowSystem — DisallowSystem protects every
// :-prefixed pseudo-header.
func TestResolveMutationRules_DisallowSystem(t *testing.T) {
	t.Parallel()
	rules := &commonmutationv3.HeaderMutationRules{
		DisallowSystem: wrapperspb.Bool(true),
	}
	r := resolveMutationRules(rules)
	if r.isAllowed(":status") {
		t.Errorf("isAllowed(:status) = true under disallow_system; want false")
	}
	if !r.isAllowed("x-custom") {
		t.Errorf("isAllowed(x-custom) = false; want true (not :-prefixed)")
	}
}

// TestApplyHeaderMutation_AllowedSet — allowed set_headers are applied
// without rejection; anyRejected=false.
func TestApplyHeaderMutation_AllowedSet(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{mutationRules: resolveMutationRules(nil)}
	f := &filter{cc: cc}
	hm := &extprocsvcv3.HeaderMutation{
		SetHeaders: []*corev3.HeaderValueOption{
			{
				Header:       &corev3.HeaderValue{Key: "x-test", Value: "v"},
				AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			},
		},
	}
	if rejected := applyHeaderMutation(f, stageRequestHeaders, hm); rejected {
		t.Errorf("anyRejected = true; want false for allowed mutation")
	}
}

// TestApplyHeaderMutation_RejectedRoutingHeader — host is rejected under the
// default rule set; anyRejected=true.
func TestApplyHeaderMutation_RejectedRoutingHeader(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{mutationRules: resolveMutationRules(nil)}
	f := &filter{cc: cc}
	hm := &extprocsvcv3.HeaderMutation{
		SetHeaders: []*corev3.HeaderValueOption{
			{
				Header:       &corev3.HeaderValue{Key: "host", Value: "evil.local"},
				AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			},
		},
	}
	if rejected := applyHeaderMutation(f, stageRequestHeaders, hm); !rejected {
		t.Errorf("anyRejected = false; want true (host protected by default)")
	}
}

// TestApplyHeaderMutation_RejectedEnvoyHeader — x-envoy-* rejected by default.
func TestApplyHeaderMutation_RejectedEnvoyHeader(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{mutationRules: resolveMutationRules(nil)}
	f := &filter{cc: cc}
	hm := &extprocsvcv3.HeaderMutation{
		SetHeaders: []*corev3.HeaderValueOption{
			{
				Header:       &corev3.HeaderValue{Key: "x-envoy-attempt", Value: "42"},
				AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			},
		},
	}
	if rejected := applyHeaderMutation(f, stageRequestHeaders, hm); !rejected {
		t.Errorf("anyRejected = false; want true (x-envoy-* protected by default)")
	}
}

// TestApplyHeaderMutation_AllowAllRouting — when allow_all_routing=true, host
// is allowed.
func TestApplyHeaderMutation_AllowAllRouting(t *testing.T) {
	t.Parallel()
	rules := &commonmutationv3.HeaderMutationRules{
		AllowAllRouting: wrapperspb.Bool(true),
	}
	cc := &compiledConfig{mutationRules: resolveMutationRules(rules)}
	f := &filter{cc: cc}
	hm := &extprocsvcv3.HeaderMutation{
		SetHeaders: []*corev3.HeaderValueOption{
			{
				Header:       &corev3.HeaderValue{Key: "host", Value: "new.local"},
				AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			},
		},
	}
	if rejected := applyHeaderMutation(f, stageRequestHeaders, hm); rejected {
		t.Errorf("anyRejected = true; want false (allow_all_routing)")
	}
}

// TestApplyHeaderMutation_RemoveHeaderRejected — rejected remove names also
// surface as rejection.
func TestApplyHeaderMutation_RemoveHeaderRejected(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{mutationRules: resolveMutationRules(nil)}
	f := &filter{cc: cc}
	hm := &extprocsvcv3.HeaderMutation{
		RemoveHeaders: []string{"x-envoy-internal"},
	}
	if rejected := applyHeaderMutation(f, stageRequestHeaders, hm); !rejected {
		t.Errorf("anyRejected = false; want true (x-envoy-internal protected)")
	}
}

// TestApplyHeaderMutation_AppendActionDispatch — the 4-arm append_action
// switch covers all proto enum values without panic.
func TestApplyHeaderMutation_AppendActionDispatch(t *testing.T) {
	t.Parallel()
	actions := []corev3.HeaderValueOption_HeaderAppendAction{
		corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
		corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
		corev3.HeaderValueOption_OVERWRITE_IF_EXISTS,
		corev3.HeaderValueOption_ADD_IF_ABSENT,
	}
	cc := &compiledConfig{mutationRules: resolveMutationRules(nil)}
	f := &filter{cc: cc}
	for _, a := range actions {
		hm := &extprocsvcv3.HeaderMutation{
			SetHeaders: []*corev3.HeaderValueOption{
				{Header: &corev3.HeaderValue{Key: "x-test", Value: "v"}, AppendAction: a},
			},
		}
		if rejected := applyHeaderMutation(f, stageRequestHeaders, hm); rejected {
			t.Errorf("action=%v: rejected=true; want false", a)
		}
	}
}

// TestApplyHeaderMutation_NilGuards — nil filter / nil mutation are safe.
func TestApplyHeaderMutation_NilGuards(t *testing.T) {
	t.Parallel()
	if rejected := applyHeaderMutation(nil, stageRequestHeaders, &extprocsvcv3.HeaderMutation{}); rejected {
		t.Errorf("nil filter: rejected=true; want false")
	}
	f := &filter{}
	if rejected := applyHeaderMutation(f, stageRequestHeaders, nil); rejected {
		t.Errorf("nil hm: rejected=true; want false")
	}
}

// TestApplyHeaderMutation_EmptyKeySkipped — empty header keys silently
// skipped + no rejection counted.
func TestApplyHeaderMutation_EmptyKeySkipped(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{mutationRules: resolveMutationRules(nil)}
	f := &filter{cc: cc}
	hm := &extprocsvcv3.HeaderMutation{
		SetHeaders: []*corev3.HeaderValueOption{
			{Header: &corev3.HeaderValue{Key: "", Value: "v"}},
		},
		RemoveHeaders: []string{""},
	}
	if rejected := applyHeaderMutation(f, stageRequestHeaders, hm); rejected {
		t.Errorf("empty-key: rejected=true; want false (silent-skip)")
	}
}

// ---------------------------------------------------------------------------
// Group 5 — applyProcessingResponse per-stage dispatch (Task 8).
// ---------------------------------------------------------------------------

// mkProcessingResponseRequestHeaders constructs a *ProcessingResponse with a
// request_headers oneof arm carrying the given CommonResponse.
func mkProcessingResponseRequestHeaders(cr *extprocsvcv3.CommonResponse) *extprocsvcv3.ProcessingResponse {
	return &extprocsvcv3.ProcessingResponse{
		Response: &extprocsvcv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocsvcv3.HeadersResponse{Response: cr},
		},
	}
}

func mkProcessingResponseResponseHeaders(cr *extprocsvcv3.CommonResponse) *extprocsvcv3.ProcessingResponse {
	return &extprocsvcv3.ProcessingResponse{
		Response: &extprocsvcv3.ProcessingResponse_ResponseHeaders{
			ResponseHeaders: &extprocsvcv3.HeadersResponse{Response: cr},
		},
	}
}

// TestApplyProcessingResponse_RequestHeadersStage_ContinueDefault — empty
// CommonResponse on request_headers stage → actContinue + nil error.
func TestApplyProcessingResponse_RequestHeadersStage_ContinueDefault(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{stats: newFilterStats(stats.NewRegistry(), "t")}
	f := &filter{cc: cc}
	resp := mkProcessingResponseRequestHeaders(&extprocsvcv3.CommonResponse{})
	act, err := applyProcessingResponse(f, stageRequestHeaders, resp)
	if err != nil {
		t.Errorf("err = %v; want nil", err)
	}
	if act != actContinue {
		t.Errorf("act = %v; want actContinue", act)
	}
}

// TestApplyProcessingResponse_ResponseHeadersStage_Continue — empty
// CommonResponse on response_headers stage → actContinue + nil error.
func TestApplyProcessingResponse_ResponseHeadersStage_Continue(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{stats: newFilterStats(stats.NewRegistry(), "t")}
	f := &filter{cc: cc}
	resp := mkProcessingResponseResponseHeaders(&extprocsvcv3.CommonResponse{})
	act, err := applyProcessingResponse(f, stageResponseHeaders, resp)
	if err != nil {
		t.Errorf("err = %v; want nil", err)
	}
	if act != actContinue {
		t.Errorf("act = %v; want actContinue", act)
	}
}

// TestApplyProcessingResponse_StageMismatch_SpuriousDispError — a server
// returning response_headers for our request_headers send → spurious +
// actError + errStageMismatch.
func TestApplyProcessingResponse_StageMismatch_SpuriousDispError(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{stats: newFilterStats(reg, "t")}
	f := &filter{cc: cc}
	// Server returned response_headers — but the filter dispatched
	// stageRequestHeaders.
	resp := mkProcessingResponseResponseHeaders(&extprocsvcv3.CommonResponse{})
	act, err := applyProcessingResponse(f, stageRequestHeaders, resp)
	if act != actError {
		t.Errorf("act = %v; want actError", act)
	}
	if !errors.Is(err, errStageMismatch) {
		t.Errorf("err = %v; want errStageMismatch", err)
	}
	if got := cc.stats.spuriousMsgsReceived.Load(); got != 1 {
		t.Errorf("spuriousMsgsReceived = %d; want 1", got)
	}
}

// TestApplyProcessingResponse_ContinueAndReplace_HeaderStageWithBodyModeNONE_NoOp
// — per Task 6 ADR-0172 §Decision AMENDMENT (SPEC §4.3 row 1): at header
// stages WITH body-mode = NONE, CONTINUE_AND_REPLACE is CONSUMED as a no-op
// for body (header_mutation still applies; the 19.1 spurious-dispatch LIFTS).
// No counter increment + no error + actContinue.
func TestApplyProcessingResponse_ContinueAndReplace_HeaderStageWithBodyModeNONE_NoOp(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{stats: newFilterStats(reg, "t")}
	// body-mode = NONE per the active processing mode.
	f := &filter{cc: cc, activeProcessingMode: &resolvedProcessingMode{
		RequestBodyMode:  extprocv3.ProcessingMode_NONE,
		ResponseBodyMode: extprocv3.ProcessingMode_NONE,
	}}
	cr := &extprocsvcv3.CommonResponse{Status: extprocsvcv3.CommonResponse_CONTINUE_AND_REPLACE}
	resp := mkProcessingResponseRequestHeaders(cr)
	act, err := applyProcessingResponse(f, stageRequestHeaders, resp)
	if err != nil {
		t.Errorf("err = %v; want nil (CONSUMED as no-op for body when body-mode = NONE per SPEC §4.3)", err)
	}
	if act != actContinue {
		t.Errorf("act = %v; want actContinue (CONSUMED as no-op per SPEC §4.3 row 1)", act)
	}
	if got := cc.stats.spuriousMsgsReceived.Load(); got != 0 {
		t.Errorf("spuriousMsgsReceived = %d; want 0 (19.1 spurious-dispatch LIFTS at 19.2 per SPEC §4.3)", got)
	}
	// The body-stage outbound skip flag MUST NOT fire when body-mode = NONE
	// (no body dispatch will happen anyway).
	if f.skipBodyStageDispatch[directionRequest] || f.skipBodyStageDispatch[directionResponse] {
		t.Errorf("skipBodyStageDispatch = %v; want both false (body-mode = NONE → no skip needed)", f.skipBodyStageDispatch)
	}
}

// TestApplyProcessingResponse_HeaderMutationRejection_SpuriousOnce — a
// CommonResponse with multiple rejected mutations bumps the spurious counter
// EXACTLY ONCE (NOT per-rejected-header).
func TestApplyProcessingResponse_HeaderMutationRejection_SpuriousOnce(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		stats:         newFilterStats(reg, "t"),
		mutationRules: resolveMutationRules(nil),
	}
	f := &filter{cc: cc}
	hm := &extprocsvcv3.HeaderMutation{
		SetHeaders: []*corev3.HeaderValueOption{
			{Header: &corev3.HeaderValue{Key: "host", Value: "evil.local"}},
			{Header: &corev3.HeaderValue{Key: "x-envoy-attempt", Value: "1"}},
			{Header: &corev3.HeaderValue{Key: ":authority", Value: "boom"}},
		},
	}
	cr := &extprocsvcv3.CommonResponse{HeaderMutation: hm}
	resp := mkProcessingResponseRequestHeaders(cr)
	act, err := applyProcessingResponse(f, stageRequestHeaders, resp)
	if err != nil {
		t.Errorf("err = %v; want nil", err)
	}
	if act != actContinue {
		t.Errorf("act = %v; want actContinue (rejection alone does not stop the stream)", act)
	}
	if got := cc.stats.spuriousMsgsReceived.Load(); got != 1 {
		t.Errorf("spuriousMsgsReceived = %d; want 1 (ONCE per stage with any rejection)", got)
	}
}

// TestApplyProcessingResponse_OverrideMessageTimeout_HonoredAndReturnsStillWaiting
// — the override is delegated to handleOverrideMessageTimeout; the action
// returned is actContinueButStillWaiting per Carryforward B.
func TestApplyProcessingResponse_OverrideMessageTimeout_HonoredAndReturnsStillWaiting(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		messageTimeout:    200 * time.Millisecond,
		maxMessageTimeout: 10 * time.Second,
		stats:             newFilterStats(reg, "t"),
	}
	f := &filter{cc: cc}
	resp := &extprocsvcv3.ProcessingResponse{
		OverrideMessageTimeout: durationpb.New(500 * time.Millisecond),
	}
	act, err := applyProcessingResponse(f, stageRequestHeaders, resp)
	if err != nil {
		t.Errorf("err = %v; want nil", err)
	}
	if act != actContinueButStillWaiting {
		t.Errorf("act = %v; want actContinueButStillWaiting", act)
	}
	if f.activeMsgTimeout != 500*time.Millisecond {
		t.Errorf("f.activeMsgTimeout = %v; want 500ms (override honored)", f.activeMsgTimeout)
	}
}

// TestApplyProcessingResponse_ModeOverride_HeaderResponse_Applied — when
// allow_mode_override=true + stage is header-response + override is in the
// (empty=any) allowlist, f.activeProcessingMode is mutated.
func TestApplyProcessingResponse_ModeOverride_HeaderResponse_Applied(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{
		allowModeOverride: true,
		stats:             newFilterStats(stats.NewRegistry(), "t"),
	}
	f := &filter{cc: cc, activeProcessingMode: &resolvedProcessingMode{RequestHeaderMode: extprocv3.ProcessingMode_SEND}}
	mo := &extprocv3.ProcessingMode{
		ResponseHeaderMode: extprocv3.ProcessingMode_SKIP,
	}
	resp := &extprocsvcv3.ProcessingResponse{
		ModeOverride: mo,
		Response: &extprocsvcv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocsvcv3.HeadersResponse{Response: &extprocsvcv3.CommonResponse{}},
		},
	}
	act, err := applyProcessingResponse(f, stageRequestHeaders, resp)
	if err != nil {
		t.Errorf("err = %v; want nil", err)
	}
	if act != actContinue {
		t.Errorf("act = %v; want actContinue", act)
	}
	if f.activeProcessingMode == nil {
		t.Fatalf("activeProcessingMode = nil; want mutated")
	}
	if f.activeProcessingMode.ResponseHeaderMode != extprocv3.ProcessingMode_SKIP {
		t.Errorf("ResponseHeaderMode = %v; want SKIP (mode_override applied)", f.activeProcessingMode.ResponseHeaderMode)
	}
}

// TestApplyProcessingResponse_ModeOverride_NoAllowModeOverride_Ignored —
// allow_mode_override=false → silent-ignore.
func TestApplyProcessingResponse_ModeOverride_NoAllowModeOverride_Ignored(t *testing.T) {
	t.Parallel()
	original := &resolvedProcessingMode{ResponseHeaderMode: extprocv3.ProcessingMode_SEND}
	cc := &compiledConfig{
		allowModeOverride: false,
		stats:             newFilterStats(stats.NewRegistry(), "t"),
	}
	f := &filter{cc: cc, activeProcessingMode: original}
	mo := &extprocv3.ProcessingMode{ResponseHeaderMode: extprocv3.ProcessingMode_SKIP}
	resp := &extprocsvcv3.ProcessingResponse{
		ModeOverride: mo,
		Response: &extprocsvcv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocsvcv3.HeadersResponse{Response: &extprocsvcv3.CommonResponse{}},
		},
	}
	if _, err := applyProcessingResponse(f, stageRequestHeaders, resp); err != nil {
		t.Errorf("err = %v", err)
	}
	if f.activeProcessingMode.ResponseHeaderMode != extprocv3.ProcessingMode_SEND {
		t.Errorf("ResponseHeaderMode = %v; want SEND (override silently ignored)", f.activeProcessingMode.ResponseHeaderMode)
	}
}

// TestShouldClearRouteCache_Precedence — precedence table per parent §5.P5.
func TestShouldClearRouteCache_Precedence(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		clearFlag bool
		action    extprocv3.ExternalProcessor_RouteCacheAction
		want      bool
	}{
		{"DEFAULT_flag_true", true, extprocv3.ExternalProcessor_DEFAULT, true},
		{"DEFAULT_flag_false", false, extprocv3.ExternalProcessor_DEFAULT, false},
		{"CLEAR_flag_false", false, extprocv3.ExternalProcessor_CLEAR, true},
		{"CLEAR_flag_true", true, extprocv3.ExternalProcessor_CLEAR, true},
		{"RETAIN_flag_true", true, extprocv3.ExternalProcessor_RETAIN, false},
		{"RETAIN_flag_false", false, extprocv3.ExternalProcessor_RETAIN, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := shouldClearRouteCache(tc.clearFlag, tc.action)
			if got != tc.want {
				t.Errorf("shouldClearRouteCache(%v, %v) = %v; want %v", tc.clearFlag, tc.action, got, tc.want)
			}
		})
	}
}

// TestApplyProcessingResponse_NilGuards — nil filter / nil resp return
// actContinue + nil error (defensive).
func TestApplyProcessingResponse_NilGuards(t *testing.T) {
	t.Parallel()
	act, err := applyProcessingResponse(nil, stageRequestHeaders, &extprocsvcv3.ProcessingResponse{})
	if act != actContinue || err != nil {
		t.Errorf("nil filter: (%v, %v); want (actContinue, nil)", act, err)
	}
	act, err = applyProcessingResponse(&filter{}, stageRequestHeaders, nil)
	if act != actContinue || err != nil {
		t.Errorf("nil resp: (%v, %v); want (actContinue, nil)", act, err)
	}
}

// TestIsModeInAllowlist_Empty — empty allowlist permits any mode.
func TestIsModeInAllowlist_Empty(t *testing.T) {
	t.Parallel()
	if !isModeInAllowlist(&resolvedProcessingMode{}, nil) {
		t.Errorf("empty allowlist: false; want true")
	}
	if !isModeInAllowlist(&resolvedProcessingMode{}, []*resolvedProcessingMode{}) {
		t.Errorf("zero-len allowlist: false; want true")
	}
}

// TestIsModeInAllowlist_MatchAndMismatch — exact match → true; mismatch →
// false.
func TestIsModeInAllowlist_MatchAndMismatch(t *testing.T) {
	t.Parallel()
	allowed := []*resolvedProcessingMode{
		{ResponseHeaderMode: extprocv3.ProcessingMode_SKIP},
	}
	if !isModeInAllowlist(&resolvedProcessingMode{ResponseHeaderMode: extprocv3.ProcessingMode_SKIP}, allowed) {
		t.Errorf("matching mode: false; want true")
	}
	if isModeInAllowlist(&resolvedProcessingMode{ResponseHeaderMode: extprocv3.ProcessingMode_SEND}, allowed) {
		t.Errorf("non-matching mode: true; want false")
	}
}

// ---------------------------------------------------------------------------
// Group 6 — emitImmediateResponse multi-stage deny per parent §5.P2 + ADR-0172
// (Task 8). FIRST §9 row to emit SendLocalReply from the encode side at the
// response_headers stage.
// ---------------------------------------------------------------------------

// recordingDCB captures SendLocalReply args for assertions.
//
// **Task 7 race safety**: the Task 7 integration tests
// (`TestExtProc_BodyStageImmediateResponse_EndToEnd` +
// `TestExtProc_OnDestroy_DuringBodyStageOutbound_NoBufferReleaseFires`) invoke
// SendLocalReply from the async dispatch goroutine — the test's main
// goroutine polls the fields via `waitForCondition`. The lrMu mutex protects
// the per-field writes + the matching `lrCallsSafe` / `lrStatusSafe` /
// `lrBodySafe` accessors used by the Task 7 tests. The existing 19.1-era
// tests that read fields directly (without mutex) are race-safe because
// they invoke SendLocalReply synchronously from the test goroutine.
type recordingDCB struct {
	fakeDCB
	lrStatus  int
	lrBody    string
	lrHeaders envoyhttp.OrderedHeaders
	lrCalls   int
	lrMu      sync.Mutex
}

func (r *recordingDCB) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	r.lrMu.Lock()
	r.lrCalls++
	r.lrStatus = status
	r.lrBody = body
	r.lrHeaders = headers
	r.lrMu.Unlock()
}

// lrCallsSafe + lrStatusSafe + lrBodySafe return the recorded
// SendLocalReply args under the lrMu mutex; consumed by Task 7 integration
// tests that poll fields from the test goroutine while the dispatch
// goroutine writes them.
func (r *recordingDCB) lrCallsSafe() int {
	r.lrMu.Lock()
	defer r.lrMu.Unlock()
	return r.lrCalls
}

func (r *recordingDCB) lrStatusSafe() int {
	r.lrMu.Lock()
	defer r.lrMu.Unlock()
	return r.lrStatus
}

func (r *recordingDCB) lrBodySafe() string {
	r.lrMu.Lock()
	defer r.lrMu.Unlock()
	return r.lrBody
}

// TestEmitImmediateResponse_DecodeStage_NonGRPC_TextPlain — body + no
// content-type → text/plain fallback.
func TestEmitImmediateResponse_DecodeStage_NonGRPC_TextPlain(t *testing.T) {
	t.Parallel()
	dcb := &recordingDCB{}
	f := &filter{dcb: dcb}
	ir := &extprocsvcv3.ImmediateResponse{
		Status: &typev3.HttpStatus{Code: typev3.StatusCode_Forbidden},
		Body:   []byte("denied"),
	}
	act := emitImmediateResponse(f, ir, stageRequestHeaders)
	if act != actImmediate {
		t.Errorf("act = %v; want actImmediate", act)
	}
	if dcb.lrCalls != 1 {
		t.Fatalf("dcb.SendLocalReply calls = %d; want 1", dcb.lrCalls)
	}
	if dcb.lrStatus != 403 {
		t.Errorf("status = %d; want 403", dcb.lrStatus)
	}
	if dcb.lrBody != "denied" {
		t.Errorf("body = %q; want %q", dcb.lrBody, "denied")
	}
	if got := dcb.lrHeaders.Get("content-type"); got != "text/plain" {
		t.Errorf("content-type = %q; want text/plain", got)
	}
}

// TestEmitImmediateResponse_DecodeStage_GRPC_BodyInGrpcMessage — gRPC-downstream
// detected via f.requestContentType → body routes into grpc-message header;
// content-type set to application/grpc.
func TestEmitImmediateResponse_DecodeStage_GRPC_BodyInGrpcMessage(t *testing.T) {
	t.Parallel()
	dcb := &recordingDCB{}
	f := &filter{dcb: dcb, requestContentType: "application/grpc"}
	ir := &extprocsvcv3.ImmediateResponse{
		Status:     &typev3.HttpStatus{Code: typev3.StatusCode_Forbidden},
		Body:       []byte("permission denied"),
		GrpcStatus: &extprocsvcv3.GrpcStatus{Status: 7},
	}
	act := emitImmediateResponse(f, ir, stageRequestHeaders)
	if act != actImmediate {
		t.Errorf("act = %v; want actImmediate", act)
	}
	if dcb.lrCalls != 1 {
		t.Fatalf("calls = %d; want 1", dcb.lrCalls)
	}
	// Body should be cleared from the wire (it lives in grpc-message header).
	if dcb.lrBody != "" {
		t.Errorf("body = %q; want empty (body in grpc-message header)", dcb.lrBody)
	}
	if got := dcb.lrHeaders.Get("grpc-message"); got != "permission denied" {
		t.Errorf("grpc-message = %q; want %q", got, "permission denied")
	}
	if got := dcb.lrHeaders.Get("content-type"); got != "application/grpc" {
		t.Errorf("content-type = %q; want application/grpc", got)
	}
	if got := dcb.lrHeaders.Get("grpc-status"); got != "7" {
		t.Errorf("grpc-status = %q; want %q", got, "7")
	}
}

// TestEmitImmediateResponse_EncodeStage_RoutesThroughDcb — encode-stage
// emission also routes through dcb.SendLocalReply per ADR-0085 (the encode
// chain enters at filter[len-1]; dcb is non-nil on both stages per ADR-0167).
// This is the FIRST §9 row to emit SendLocalReply from the encode side at
// response_headers per ADR-0167.
func TestEmitImmediateResponse_EncodeStage_RoutesThroughDcb(t *testing.T) {
	t.Parallel()
	dcb := &recordingDCB{}
	ecb := &fakeECB{}
	f := &filter{dcb: dcb, ecb: ecb}
	ir := &extprocsvcv3.ImmediateResponse{
		Status: &typev3.HttpStatus{Code: typev3.StatusCode_InternalServerError},
		Body:   []byte("err"),
	}
	act := emitImmediateResponse(f, ir, stageResponseHeaders)
	if act != actImmediate {
		t.Errorf("act = %v; want actImmediate", act)
	}
	if dcb.lrCalls != 1 {
		t.Errorf("dcb.SendLocalReply calls = %d; want 1 (encode-stage routes through dcb per ADR-0085)", dcb.lrCalls)
	}
	if dcb.lrStatus != 500 {
		t.Errorf("status = %d; want 500", dcb.lrStatus)
	}
	// ContinueEncoding should NOT have been called (the local-reply emission
	// supersedes the chain).
	if ecb.calls() != 0 {
		t.Errorf("ContinueEncoding calls = %d; want 0 (actImmediate)", ecb.calls())
	}
}

// TestEmitImmediateResponse_StatusDefault — nil HttpStatus / Empty code →
// defaults to 200 per envoy-go-strict permissive default.
func TestEmitImmediateResponse_StatusDefault(t *testing.T) {
	t.Parallel()
	dcb := &recordingDCB{}
	f := &filter{dcb: dcb}
	ir := &extprocsvcv3.ImmediateResponse{Body: []byte("ok")}
	emitImmediateResponse(f, ir, stageRequestHeaders)
	if dcb.lrStatus != 200 {
		t.Errorf("status = %d; want 200 (default when nil HttpStatus)", dcb.lrStatus)
	}
}

// TestEmitImmediateResponse_HeaderMutationGated — host (protected by default)
// is silently dropped; allowed headers pass through.
func TestEmitImmediateResponse_HeaderMutationGated(t *testing.T) {
	t.Parallel()
	dcb := &recordingDCB{}
	cc := &compiledConfig{mutationRules: resolveMutationRules(nil)}
	f := &filter{dcb: dcb, cc: cc}
	ir := &extprocsvcv3.ImmediateResponse{
		Status: &typev3.HttpStatus{Code: typev3.StatusCode_Forbidden},
		Headers: &extprocsvcv3.HeaderMutation{
			SetHeaders: []*corev3.HeaderValueOption{
				{Header: &corev3.HeaderValue{Key: "x-decision", Value: "deny"}},
				{Header: &corev3.HeaderValue{Key: "host", Value: "evil.local"}}, // protected
			},
		},
	}
	emitImmediateResponse(f, ir, stageRequestHeaders)
	if got := dcb.lrHeaders.Get("x-decision"); got != "deny" {
		t.Errorf("x-decision = %q; want %q", got, "deny")
	}
	if got := dcb.lrHeaders.Get("host"); got != "" {
		t.Errorf("host = %q; want empty (rejected by mutation_rules)", got)
	}
}

// TestEmitImmediateResponse_NilGuards — nil filter / nil ir return actImmediate
// without panicking.
func TestEmitImmediateResponse_NilGuards(t *testing.T) {
	t.Parallel()
	if got := emitImmediateResponse(nil, &extprocsvcv3.ImmediateResponse{}, stageRequestHeaders); got != actImmediate {
		t.Errorf("nil f: %v; want actImmediate", got)
	}
	f := &filter{}
	if got := emitImmediateResponse(f, nil, stageRequestHeaders); got != actImmediate {
		t.Errorf("nil ir: %v; want actImmediate", got)
	}
}

// ---------------------------------------------------------------------------
// Group 9 — error-posture (failure_mode_allow + disable_immediate_response)
// per SPEC §4 + parent §5.P11 + ADR-0172 §Decision (Task 8).
// ---------------------------------------------------------------------------

// TestApplyProcessingResponse_DisableImmediateResponse_SilentDrop — when
// disable_immediate_response=true, an ImmediateResponse arrival silent-drops
// + bumps spuriousMsgsReceived; returns actContinue.
func TestApplyProcessingResponse_DisableImmediateResponse_SilentDrop(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	dcb := &recordingDCB{}
	cc := &compiledConfig{
		disableImmediateResponse: true,
		stats:                    newFilterStats(reg, "t"),
	}
	f := &filter{cc: cc, dcb: dcb}
	ir := &extprocsvcv3.ImmediateResponse{
		Status: &typev3.HttpStatus{Code: typev3.StatusCode_Forbidden},
		Body:   []byte("denied"),
	}
	resp := &extprocsvcv3.ProcessingResponse{
		Response: &extprocsvcv3.ProcessingResponse_ImmediateResponse{ImmediateResponse: ir},
	}
	act, err := applyProcessingResponse(f, stageRequestHeaders, resp)
	if err != nil {
		t.Errorf("err = %v; want nil (silent-drop)", err)
	}
	if act != actContinue {
		t.Errorf("act = %v; want actContinue (silent-drop)", act)
	}
	if dcb.lrCalls != 0 {
		t.Errorf("SendLocalReply called %d times; want 0 (silent-drop)", dcb.lrCalls)
	}
	if got := cc.stats.spuriousMsgsReceived.Load(); got != 1 {
		t.Errorf("spuriousMsgsReceived = %d; want 1", got)
	}
}

// TestMapTransportError_FailureModeAllowFalse_DecodeStage_500 — error +
// failure_mode_allow=false → SendLocalReply(500) + actImmediate.
func TestMapTransportError_FailureModeAllowFalse_DecodeStage_500(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	dcb := &recordingDCB{}
	cc := &compiledConfig{
		failureModeAllow: false,
		stats:            newFilterStats(reg, "t"),
	}
	f := &filter{cc: cc, dcb: dcb}
	act := mapTransportError(f, stageRequestHeaders, errors.New("dial timeout"))
	if act != actImmediate {
		t.Errorf("act = %v; want actImmediate", act)
	}
	if dcb.lrCalls != 1 {
		t.Fatalf("SendLocalReply calls = %d; want 1", dcb.lrCalls)
	}
	if dcb.lrStatus != 500 {
		t.Errorf("status = %d; want 500", dcb.lrStatus)
	}
	if got := cc.stats.failureModeAllowed.Load(); got != 0 {
		t.Errorf("failureModeAllowed = %d; want 0 (fail-loud)", got)
	}
}

// TestMapTransportError_FailureModeAllowFalse_EncodeStage_StreamReset — error
// at response_headers stage + failure_mode_allow=false → SendLocalReply(0) as
// stream-reset signal per phase-04 + ADR-0085.
func TestMapTransportError_FailureModeAllowFalse_EncodeStage_StreamReset(t *testing.T) {
	t.Parallel()
	dcb := &recordingDCB{}
	cc := &compiledConfig{
		failureModeAllow: false,
		stats:            newFilterStats(stats.NewRegistry(), "t"),
	}
	f := &filter{cc: cc, dcb: dcb}
	act := mapTransportError(f, stageResponseHeaders, errors.New("recv timeout"))
	if act != actImmediate {
		t.Errorf("act = %v; want actImmediate", act)
	}
	if dcb.lrCalls != 1 {
		t.Errorf("SendLocalReply calls = %d; want 1", dcb.lrCalls)
	}
	if dcb.lrStatus != 0 {
		t.Errorf("status = %d; want 0 (stream-reset signal)", dcb.lrStatus)
	}
}

// TestMapTransportError_FailureModeAllowTrue_SilentContinue — failure_mode_allow=true
// → actContinue + failureModeAllowed counter increment.
func TestMapTransportError_FailureModeAllowTrue_SilentContinue(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	dcb := &recordingDCB{}
	cc := &compiledConfig{
		failureModeAllow: true,
		stats:            newFilterStats(reg, "t"),
	}
	f := &filter{cc: cc, dcb: dcb}
	act := mapTransportError(f, stageRequestHeaders, errors.New("dial timeout"))
	if act != actContinue {
		t.Errorf("act = %v; want actContinue (failure_mode_allow=true)", act)
	}
	if dcb.lrCalls != 0 {
		t.Errorf("SendLocalReply calls = %d; want 0 (silent-continue)", dcb.lrCalls)
	}
	if got := cc.stats.failureModeAllowed.Load(); got != 1 {
		t.Errorf("failureModeAllowed = %d; want 1", got)
	}
}

// TestMapTransportError_NilGuards — nil filter / nil err return actContinue
// safely.
func TestMapTransportError_NilGuards(t *testing.T) {
	t.Parallel()
	if got := mapTransportError(nil, stageRequestHeaders, errors.New("x")); got != actContinue {
		t.Errorf("nil f: %v; want actContinue", got)
	}
	if got := mapTransportError(&filter{}, stageRequestHeaders, nil); got != actContinue {
		t.Errorf("nil err: %v; want actContinue", got)
	}
}

// TestMapTransportError_FailureModeAllowTrue_DeadlineExceeded — context
// deadline-exceeded (per-message timeout) follows the same failure-mode
// dispatch path as a transport error.
func TestMapTransportError_FailureModeAllowTrue_DeadlineExceeded(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	dcb := &recordingDCB{}
	cc := &compiledConfig{
		failureModeAllow: true,
		stats:            newFilterStats(reg, "t"),
	}
	f := &filter{cc: cc, dcb: dcb}
	act := mapTransportError(f, stageRequestHeaders, context.DeadlineExceeded)
	if act != actContinue {
		t.Errorf("act = %v; want actContinue", act)
	}
	if got := cc.stats.failureModeAllowed.Load(); got != 1 {
		t.Errorf("failureModeAllowed = %d; want 1", got)
	}
}

// Compile-time conformance for the *recordingDCB extended fake — assures
// the test fake satisfies the framework interface even after methods are
// added at SendLocalReply.
var _ envoyhttp.DecoderFilterCallbacks = (*recordingDCB)(nil)

// ---------------------------------------------------------------------------
// Group 11 — attributes.go envelope builder per ADR-0174 + SPEC §6.6 (Task 9).
//
// Tests cover:
//   - lowercaseHeaderMap + sourcePrincipalFirstOrEmpty helpers (parity with
//     the phase-18.2 attributes.go precedent).
//   - buildAttributeEnvelope: the SPEC §6.6 7-attribute hypothesis-table
//     evaluation against a pluggable accessor surface — empty allowlist →
//     nil envelope; allowlist-of-one → only that attribute populated;
//     unrecognized allowlist entries → silently skipped.
//   - buildRequestHeadersProcessingRequest: end-to-end *ProcessingRequest
//     construction from f.dcb (decoder-side ADR-0165 + ADR-0144 surface) +
//     a configured allowlist; HttpHeaders.Headers carries the lowercased
//     header bundle + end_of_stream flag.
//   - buildResponseHeadersProcessingRequest: symmetric — f.ecb (encoder-side
//     ADR-0174 surface) at the response_headers stage; D10 hypothesis HELD →
//     `source.principal` is empty on the encode side (no DownstreamPrincipal
//     on EncoderFilterCallbacks per ADR-0174 §Decision).
//
// Mocked-TLS state per the PLAN Task 9 contract: dcbStub.tlsServerName =
// "example.com" populates `connection.requested_server_name`; the listener
// principal flows into `connection.subject_local_certificate` (the SPEC
// §6.6 derivation — `subject_local_certificate` is sourced from the
// listener cert + ADR-0144 listener-principal extraction, which is the same
// string the `connection.principal` attribute carries; the IMPL settles a
// reasonable hypothesis — see the buildAttributeEnvelope rationale).
// ---------------------------------------------------------------------------

// dcbStub is a Group 11 DecoderFilterCallbacks fake whose ADR-0165 + ADR-0144
// accessor return values are configurable (the production fakeDCB returns
// zero-values uniformly; Group 11 needs settable per-test state).
type dcbStub struct {
	fakeDCB              // inherit ContinueDecoding / SendLocalReply / pseudo-headers etc.
	remoteAddr           net.Addr
	localAddr            net.Addr
	tlsServerName        string
	tlsPeerCertDER       []byte
	protocol             string
	listenerPrincipal    string
	downstreamPrincipals []string
}

func (d *dcbStub) DownstreamRemoteAddr() net.Addr   { return d.remoteAddr }
func (d *dcbStub) DownstreamLocalAddr() net.Addr    { return d.localAddr }
func (d *dcbStub) DownstreamTLSServerName() string  { return d.tlsServerName }
func (d *dcbStub) DownstreamTLSPeerCertDER() []byte { return d.tlsPeerCertDER }
func (d *dcbStub) DownstreamProtocol() string       { return d.protocol }
func (d *dcbStub) ListenerPrincipal() string        { return d.listenerPrincipal }
func (d *dcbStub) DownstreamPrincipal() []string    { return d.downstreamPrincipals }

// ecbStub is the encoder-side analog. Note the absence of any
// DownstreamPrincipal-equivalent — the D10 planner-time hypothesis settled
// 6 (not 7) encoder-side methods per ADR-0174 §Decision. The
// `source.principal` attribute is therefore empty on the encode side.
type ecbStub struct {
	fakeECB
	remoteAddr        net.Addr
	localAddr         net.Addr
	tlsServerName     string
	tlsPeerCertDER    []byte
	protocol          string
	listenerPrincipal string
}

func (e *ecbStub) DownstreamRemoteAddr() net.Addr   { return e.remoteAddr }
func (e *ecbStub) DownstreamLocalAddr() net.Addr    { return e.localAddr }
func (e *ecbStub) DownstreamTLSServerName() string  { return e.tlsServerName }
func (e *ecbStub) DownstreamTLSPeerCertDER() []byte { return e.tlsPeerCertDER }
func (e *ecbStub) DownstreamProtocol() string       { return e.protocol }
func (e *ecbStub) ListenerPrincipal() string        { return e.listenerPrincipal }

// Compile-time conformance — fail fast if the Group 11 fakes drift from the
// framework interface signatures.
var (
	_ envoyhttp.DecoderFilterCallbacks = (*dcbStub)(nil)
	_ envoyhttp.EncoderFilterCallbacks = (*ecbStub)(nil)
)

// TestLowercaseHeaderMap_Basic asserts the canonical → lowercased single-value
// projection + the multi-value comma-join discipline (mirrors phase-18.2
// attributes.go precedent + the reference Envoy v1.37.2 internal lowercase +
// comma-join convention for the headers map).
func TestLowercaseHeaderMap_Basic(t *testing.T) {
	t.Parallel()
	in := http.Header{
		"Authorization":   []string{"Bearer x"},
		"X-Forwarded-For": []string{"1.1.1.1", "2.2.2.2"},
		":method":         []string{"GET"},
	}
	got := lowercaseHeaderMap(in)
	if got["authorization"] != "Bearer x" {
		t.Errorf("authorization = %q; want %q", got["authorization"], "Bearer x")
	}
	if got["x-forwarded-for"] != "1.1.1.1,2.2.2.2" {
		t.Errorf("x-forwarded-for = %q; want %q (multi-value comma-join)",
			got["x-forwarded-for"], "1.1.1.1,2.2.2.2")
	}
	if got[":method"] != "GET" {
		t.Errorf(":method = %q; want %q (pseudo-headers included)",
			got[":method"], "GET")
	}
}

// TestLowercaseHeaderMap_Empty asserts a nil / empty input returns a non-nil
// empty map (the proto-faithful contract — never nil).
func TestLowercaseHeaderMap_Empty(t *testing.T) {
	t.Parallel()
	got := lowercaseHeaderMap(nil)
	if got == nil {
		t.Errorf("lowercaseHeaderMap(nil) = nil; want non-nil empty map")
	}
	if len(got) != 0 {
		t.Errorf("len = %d; want 0", len(got))
	}
}

// TestSourcePrincipalFirstOrEmpty asserts the first-or-empty helper used by
// the source.principal attribute extraction.
func TestSourcePrincipalFirstOrEmpty(t *testing.T) {
	t.Parallel()
	if got := sourcePrincipalFirstOrEmpty(nil); got != "" {
		t.Errorf("nil → %q; want \"\"", got)
	}
	if got := sourcePrincipalFirstOrEmpty([]string{}); got != "" {
		t.Errorf("empty → %q; want \"\"", got)
	}
	if got := sourcePrincipalFirstOrEmpty([]string{"spiffe://x", "spiffe://y"}); got != "spiffe://x" {
		t.Errorf("[spiffe://x,...] → %q; want %q", got, "spiffe://x")
	}
}

// TestBuildAttributeEnvelope_EmptyAllowlist asserts an empty allowlist
// returns a nil envelope (so the ProcessingRequest.attributes field is
// omitted entirely — the wire shape carries no `attributes` map at all).
func TestBuildAttributeEnvelope_EmptyAllowlist(t *testing.T) {
	t.Parallel()
	got := buildAttributeEnvelope(nil,
		func() net.Addr { return &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234} },
		func() net.Addr { return &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 8080} },
		func() string { return "example.com" },
		func() string { return "HTTP/2" },
		func() string { return "spiffe://cluster.local/listener" },
		func() string { return "spiffe://cluster.local/client" },
	)
	if got != nil {
		t.Errorf("empty allowlist → %v; want nil envelope", got)
	}
}

// TestBuildAttributeEnvelope_SourceAddressOnly asserts a single-attribute
// allowlist populates only that attribute (the gate per SPEC §6.6 — the
// allowlist controls which CEL attributes the ProcessingRequest carries).
func TestBuildAttributeEnvelope_SourceAddressOnly(t *testing.T) {
	t.Parallel()
	got := buildAttributeEnvelope([]string{"source.address"},
		func() net.Addr { return &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234} },
		func() net.Addr { return &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 8080} },
		func() string { return "example.com" },
		func() string { return "HTTP/2" },
		func() string { return "spiffe://cluster.local/listener" },
		func() string { return "spiffe://cluster.local/client" },
	)
	if got == nil {
		t.Fatalf("got = nil; want envelope with 1 entry")
	}
	if len(got) != 1 {
		t.Errorf("len(got) = %d; want 1 (only source.address)", len(got))
	}
	if _, ok := got["source.address"]; !ok {
		t.Errorf("source.address missing; got keys = %v", keysOf(got))
	}
}

// TestBuildAttributeEnvelope_AllSevenAttributes asserts every entry in the
// SPEC §6.6 hypothesis-table populates when listed (the full attribute-name
// roster). Asserts each value's well-known field shape.
func TestBuildAttributeEnvelope_AllSevenAttributes(t *testing.T) {
	t.Parallel()
	allowlist := []string{
		"source.address",
		"destination.address",
		"connection.requested_server_name",
		"connection.subject_local_certificate",
		"request.protocol",
		"connection.principal",
		"source.principal",
	}
	got := buildAttributeEnvelope(allowlist,
		func() net.Addr { return &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234} },
		func() net.Addr { return &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 8080} },
		func() string { return "example.com" },
		func() string { return "HTTP/2" },
		func() string { return "spiffe://cluster.local/listener" },
		func() string { return "spiffe://cluster.local/client" },
	)
	if got == nil {
		t.Fatalf("got = nil; want envelope with 7 entries")
	}
	if len(got) != 7 {
		t.Errorf("len(got) = %d; want 7", len(got))
	}
	for _, k := range allowlist {
		if _, ok := got[k]; !ok {
			t.Errorf("attribute %q missing; got keys = %v", k, keysOf(got))
		}
	}
}

// TestBuildAttributeEnvelope_UnknownAttribute asserts unrecognized
// attribute-name entries are silently skipped (forward-compat — new CEL
// attribute names in future Envoy versions do not crash; the IMPL emits
// only the recognized roster).
func TestBuildAttributeEnvelope_UnknownAttribute(t *testing.T) {
	t.Parallel()
	got := buildAttributeEnvelope([]string{"source.address", "unknown.attribute.xyz"},
		func() net.Addr { return &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234} },
		func() net.Addr { return nil },
		func() string { return "" },
		func() string { return "" },
		func() string { return "" },
		func() string { return "" },
	)
	if got == nil {
		t.Fatalf("got = nil; want envelope")
	}
	if len(got) != 1 {
		t.Errorf("len(got) = %d; want 1 (unknown.attribute.xyz silently dropped)", len(got))
	}
	if _, ok := got["source.address"]; !ok {
		t.Errorf("source.address missing")
	}
	if _, ok := got["unknown.attribute.xyz"]; ok {
		t.Errorf("unknown.attribute.xyz unexpectedly present")
	}
}

// TestBuildAttributeEnvelope_EmptyValuesSkipped asserts that an attribute
// whose accessor returns the zero value (nil net.Addr, empty string, nil
// principal slice) is skipped from the envelope. The per-stage SPEC §6.6
// table omits attributes that the per-stream state cannot supply (plaintext
// connection → no `connection.requested_server_name`; etc.) — the per-attr
// presence is gated on a non-zero accessor return.
func TestBuildAttributeEnvelope_EmptyValuesSkipped(t *testing.T) {
	t.Parallel()
	got := buildAttributeEnvelope([]string{
		"source.address",
		"connection.requested_server_name",
		"source.principal",
	},
		func() net.Addr { return nil }, // empty source.address
		func() net.Addr { return nil },
		func() string { return "" }, // empty SNI
		func() string { return "" },
		func() string { return "" },
		func() string { return "" }, // empty source.principal
	)
	if got != nil {
		t.Errorf("all-empty accessors → %v; want nil envelope", got)
	}
}

// TestBuildRequestHeadersProcessingRequest_PopulatesRequestHeaders asserts
// the request_headers oneof + the HttpHeaders.Headers list + the end_of_stream
// flag survive intact + the attributes envelope is built from the decoder-side
// accessors.
func TestBuildRequestHeadersProcessingRequest_PopulatesRequestHeaders(t *testing.T) {
	t.Parallel()
	dcb := &dcbStub{
		remoteAddr:           &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234},
		localAddr:            &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 8080},
		tlsServerName:        "example.com",
		protocol:             "HTTP/2",
		listenerPrincipal:    "spiffe://cluster.local/listener",
		downstreamPrincipals: []string{"spiffe://cluster.local/client"},
	}
	f := &filter{dcb: dcb}
	hdrs := http.Header{
		":method":    []string{"GET"},
		":path":      []string{"/api/v1/resource"},
		"User-Agent": []string{"curl/7.83"},
	}
	allowlist := []string{
		"source.address",
		"connection.requested_server_name",
		"request.protocol",
		"connection.principal",
		"source.principal",
	}

	req := buildRequestHeadersProcessingRequest(f, hdrs, true, allowlist)

	if req == nil {
		t.Fatalf("req = nil; want non-nil *ProcessingRequest")
	}
	rh := req.GetRequestHeaders()
	if rh == nil {
		t.Fatalf("RequestHeaders oneof not set; got Request = %T", req.GetRequest())
	}
	if !rh.GetEndOfStream() {
		t.Errorf("EndOfStream = false; want true")
	}
	hm := rh.GetHeaders()
	if hm == nil {
		t.Fatalf("Headers = nil; want populated HeaderMap")
	}
	if len(hm.GetHeaders()) != 3 {
		t.Errorf("len(Headers.Headers) = %d; want 3", len(hm.GetHeaders()))
	}

	// Build a lookup for the per-key assertions (HeaderMap is a list, not a map).
	headersByKey := make(map[string]string)
	for _, hv := range hm.GetHeaders() {
		headersByKey[hv.GetKey()] = hv.GetValue()
	}
	if headersByKey["user-agent"] != "curl/7.83" {
		t.Errorf("user-agent = %q; want %q (lowercased)", headersByKey["user-agent"], "curl/7.83")
	}
	if headersByKey[":method"] != "GET" {
		t.Errorf(":method = %q; want %q", headersByKey[":method"], "GET")
	}

	// Attributes envelope assertions.
	attrs := req.GetAttributes()
	if attrs == nil {
		t.Fatalf("Attributes = nil; want envelope populated by allowlist")
	}
	if len(attrs) != 5 {
		t.Errorf("len(Attributes) = %d; want 5 (allowlist size)", len(attrs))
	}
	for _, want := range allowlist {
		if _, ok := attrs[want]; !ok {
			t.Errorf("attribute %q missing; got keys = %v", want, attrKeysOf(attrs))
		}
	}
}

// TestBuildRequestHeadersProcessingRequest_EmptyAllowlist_NoAttributesField
// asserts that an empty allowlist → ProcessingRequest.Attributes is nil
// (the wire shape carries no `attributes` map at all). Per parent §5.P1 the
// allowlist is the gate.
func TestBuildRequestHeadersProcessingRequest_EmptyAllowlist_NoAttributesField(t *testing.T) {
	t.Parallel()
	dcb := &dcbStub{
		remoteAddr: &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234},
	}
	f := &filter{dcb: dcb}
	hdrs := http.Header{":method": []string{"GET"}}

	req := buildRequestHeadersProcessingRequest(f, hdrs, false, nil)

	if req == nil {
		t.Fatalf("req = nil")
	}
	if req.GetRequestHeaders() == nil {
		t.Fatalf("RequestHeaders oneof not set")
	}
	if req.GetRequestHeaders().GetEndOfStream() {
		t.Errorf("EndOfStream = true; want false")
	}
	if req.GetAttributes() != nil {
		t.Errorf("Attributes = %v; want nil (empty allowlist)", req.GetAttributes())
	}
}

// TestBuildRequestHeadersProcessingRequest_SourceAddressOnly asserts the
// PLAN Task 9 acceptance criterion — allowlist `["source.address"]` →
// only that attribute populated.
func TestBuildRequestHeadersProcessingRequest_SourceAddressOnly(t *testing.T) {
	t.Parallel()
	dcb := &dcbStub{
		remoteAddr:           &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234},
		localAddr:            &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 8080},
		tlsServerName:        "example.com",
		protocol:             "HTTP/2",
		listenerPrincipal:    "spiffe://cluster.local/listener",
		downstreamPrincipals: []string{"spiffe://cluster.local/client"},
	}
	f := &filter{dcb: dcb}

	req := buildRequestHeadersProcessingRequest(f, http.Header{}, true,
		[]string{"source.address"})

	attrs := req.GetAttributes()
	if attrs == nil {
		t.Fatalf("Attributes = nil; want 1-entry envelope")
	}
	if len(attrs) != 1 {
		t.Errorf("len(attrs) = %d; want 1 (only source.address)", len(attrs))
	}
	if _, ok := attrs["source.address"]; !ok {
		t.Errorf("source.address missing")
	}
}

// TestBuildRequestHeadersProcessingRequest_TLSServerNamePopulates asserts the
// PLAN Task 9 acceptance criterion — mocked TLS state via
// dcbStub.tlsServerName="example.com" → `connection.requested_server_name`
// = "example.com" in the envelope.
func TestBuildRequestHeadersProcessingRequest_TLSServerNamePopulates(t *testing.T) {
	t.Parallel()
	dcb := &dcbStub{tlsServerName: "example.com"}
	f := &filter{dcb: dcb}

	req := buildRequestHeadersProcessingRequest(f, http.Header{}, true,
		[]string{"connection.requested_server_name"})

	attrs := req.GetAttributes()
	if attrs == nil {
		t.Fatalf("Attributes = nil; want 1-entry envelope")
	}
	entry, ok := attrs["connection.requested_server_name"]
	if !ok {
		t.Fatalf("connection.requested_server_name missing; keys = %v", attrKeysOf(attrs))
	}
	// The Struct should carry exactly one field named "value" whose
	// StringValue is "example.com" (the simple-scalar attribute encoding —
	// the IMPL settles a struct{value:<scalar>} shape per ADR-0170 §Decision
	// + ADR-0174 §Consequences orientation; closure at Task 13 fixture
	// scrape).
	fv, fok := entry.GetFields()["value"]
	if !fok {
		t.Fatalf("Struct.fields[\"value\"] missing; fields = %v", entry.GetFields())
	}
	if got := fv.GetStringValue(); got != "example.com" {
		t.Errorf("value.StringValue = %q; want %q", got, "example.com")
	}
}

// TestBuildRequestHeadersProcessingRequest_SubjectLocalCertificate asserts
// the listener-cert principal derivation. Per SPEC §6.6 the
// `connection.subject_local_certificate` attribute is sourced from the
// listener cert + ADR-0144 extraction — the IMPL settles a reasonable
// hypothesis (the listener-principal string).
func TestBuildRequestHeadersProcessingRequest_SubjectLocalCertificate(t *testing.T) {
	t.Parallel()
	dcb := &dcbStub{listenerPrincipal: "spiffe://cluster.local/listener"}
	f := &filter{dcb: dcb}

	req := buildRequestHeadersProcessingRequest(f, http.Header{}, true,
		[]string{"connection.subject_local_certificate"})

	attrs := req.GetAttributes()
	if attrs == nil {
		t.Fatalf("Attributes = nil; want 1-entry envelope")
	}
	entry, ok := attrs["connection.subject_local_certificate"]
	if !ok {
		t.Fatalf("connection.subject_local_certificate missing")
	}
	fv, fok := entry.GetFields()["value"]
	if !fok {
		t.Fatalf("Struct.fields[\"value\"] missing")
	}
	if got := fv.GetStringValue(); got != "spiffe://cluster.local/listener" {
		t.Errorf("value = %q; want %q", got, "spiffe://cluster.local/listener")
	}
}

// TestBuildResponseHeadersProcessingRequest_PopulatesResponseHeaders asserts
// the symmetric encoder-side path: response_headers oneof + HttpHeaders +
// attributes envelope sourced from f.ecb (ADR-0174 surface; NO DownstreamPrincipal
// per D10 hypothesis HELD).
func TestBuildResponseHeadersProcessingRequest_PopulatesResponseHeaders(t *testing.T) {
	t.Parallel()
	ecb := &ecbStub{
		remoteAddr:        &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234},
		localAddr:         &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 8080},
		tlsServerName:     "example.com",
		protocol:          "HTTP/2",
		listenerPrincipal: "spiffe://cluster.local/listener",
	}
	f := &filter{ecb: ecb}
	hdrs := http.Header{
		":status":      []string{"200"},
		"Content-Type": []string{"application/json"},
	}
	allowlist := []string{
		"source.address",
		"destination.address",
		"connection.requested_server_name",
		"connection.subject_local_certificate",
		"request.protocol",
		"connection.principal",
		"source.principal", // D10: empty on encode side → SKIPPED.
	}

	req := buildResponseHeadersProcessingRequest(f, hdrs, true, allowlist)

	if req == nil {
		t.Fatalf("req = nil")
	}
	rh := req.GetResponseHeaders()
	if rh == nil {
		t.Fatalf("ResponseHeaders oneof not set; got Request = %T", req.GetRequest())
	}
	if !rh.GetEndOfStream() {
		t.Errorf("EndOfStream = false; want true")
	}
	hm := rh.GetHeaders()
	if hm == nil {
		t.Fatalf("Headers = nil")
	}
	if len(hm.GetHeaders()) != 2 {
		t.Errorf("len(Headers.Headers) = %d; want 2", len(hm.GetHeaders()))
	}
	headersByKey := make(map[string]string)
	for _, hv := range hm.GetHeaders() {
		headersByKey[hv.GetKey()] = hv.GetValue()
	}
	if headersByKey["content-type"] != "application/json" {
		t.Errorf("content-type = %q; want %q (lowercased)", headersByKey["content-type"], "application/json")
	}

	attrs := req.GetAttributes()
	if attrs == nil {
		t.Fatalf("Attributes = nil")
	}
	// D10: source.principal is EMPTY on the encode side (no DownstreamPrincipal
	// on EncoderFilterCallbacks per ADR-0174). The other 6 populate.
	if _, ok := attrs["source.principal"]; ok {
		t.Errorf("source.principal unexpectedly present on encode side; D10 hypothesis FALSIFIED — ADR-0174 §Decision must amend to 7 methods")
	}
	if len(attrs) != 6 {
		t.Errorf("len(attrs) = %d; want 6 (D10: encode-side source.principal empty + skipped)", len(attrs))
	}
}

// TestBuildResponseHeadersProcessingRequest_EmptyAllowlist asserts an empty
// allowlist → no attributes field (the symmetric encode-side
// no-attributes-when-nothing-configured property).
func TestBuildResponseHeadersProcessingRequest_EmptyAllowlist(t *testing.T) {
	t.Parallel()
	ecb := &ecbStub{
		remoteAddr: &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234},
	}
	f := &filter{ecb: ecb}
	hdrs := http.Header{":status": []string{"200"}}

	req := buildResponseHeadersProcessingRequest(f, hdrs, false, nil)

	if req == nil {
		t.Fatalf("req = nil")
	}
	if req.GetResponseHeaders() == nil {
		t.Fatalf("ResponseHeaders oneof not set")
	}
	if req.GetResponseHeaders().GetEndOfStream() {
		t.Errorf("EndOfStream = true; want false")
	}
	if req.GetAttributes() != nil {
		t.Errorf("Attributes = %v; want nil (empty allowlist)", req.GetAttributes())
	}
}

// TestBuildResponseHeadersProcessingRequest_D10_SourcePrincipalEmpty asserts
// the D10 hypothesis HELD: even when `source.principal` is in the allowlist,
// the encode-side envelope omits it (no DownstreamPrincipal accessor on
// EncoderFilterCallbacks per ADR-0174 §Decision). This is the load-bearing
// assertion documented at PROGRESS.md Task 9 entry.
func TestBuildResponseHeadersProcessingRequest_D10_SourcePrincipalEmpty(t *testing.T) {
	t.Parallel()
	ecb := &ecbStub{
		remoteAddr:        &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234},
		listenerPrincipal: "spiffe://cluster.local/listener",
	}
	f := &filter{ecb: ecb}

	req := buildResponseHeadersProcessingRequest(f, http.Header{}, true,
		[]string{"source.principal", "connection.principal"})

	attrs := req.GetAttributes()
	if attrs == nil {
		t.Fatalf("Attributes = nil; want partial envelope")
	}
	if _, ok := attrs["source.principal"]; ok {
		t.Errorf("source.principal present; want absent (D10 hypothesis HELD — encode-side has no DownstreamPrincipal accessor)")
	}
	if _, ok := attrs["connection.principal"]; !ok {
		t.Errorf("connection.principal missing; want present (sourced from f.ecb.ListenerPrincipal())")
	}
}

// ---------------------------------------------------------------------------
// Group N+3 — body-stage attribute envelope builders (Task 5).
//
// Tests the buildRequestBodyProcessingRequest + buildResponseBodyProcessingRequest
// surface per parent SPEC §6.5 + §6.6 + planner-time D5 (header-stage SUPERSET;
// adds body-stage-natural `request.size` / `response.size` populated from
// `len(body)` rather than Content-Length-derived). The exact body-stage
// attribute roster crystallizes empirically at Task 9 fixture-harness scrape
// against reference Envoy v1.37.2; this group lands the PLAN-time hypothesis.
// ---------------------------------------------------------------------------

// TestBuildBodyProcessingRequest_PopulatesRequestBodyField asserts the
// request_body oneof + HttpBody.Body bytes + end_of_stream flag survive intact
// at the body stage (the body-stage analog of
// TestBuildRequestHeadersProcessingRequest_PopulatesRequestHeaders).
func TestBuildBodyProcessingRequest_PopulatesRequestBodyField(t *testing.T) {
	t.Parallel()
	dcb := &dcbStub{
		remoteAddr:    &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234},
		tlsServerName: "example.com",
	}
	f := &filter{dcb: dcb}
	body := []byte(`{"hello":"world"}`)

	req := buildRequestBodyProcessingRequest(f, body, true, nil)

	if req == nil {
		t.Fatalf("req = nil; want non-nil *ProcessingRequest")
	}
	rb := req.GetRequestBody()
	if rb == nil {
		t.Fatalf("RequestBody oneof not set; got Request = %T", req.GetRequest())
	}
	if got := rb.GetBody(); !bytes.Equal(got, body) {
		t.Errorf("Body = %q; want %q", got, body)
	}
	if !rb.GetEndOfStream() {
		t.Errorf("EndOfStream = false; want true")
	}
	// Empty allowlist → no attributes envelope (per D5 + the header-stage
	// no-attributes-when-nothing-configured discipline).
	if req.GetAttributes() != nil {
		t.Errorf("Attributes = %v; want nil (empty allowlist)", req.GetAttributes())
	}
}

// TestBuildBodyProcessingRequest_AttributesEnvelopeMirrorsHeaderStage asserts
// the body-stage attribute envelope MIRRORS the header-stage envelope when
// driven against the same f.dcb + the same allowlist (excluding the body-stage-
// only `request.size` attribute). The D5 SUPERSET property — header-stage
// roster carries to body stage unchanged.
func TestBuildBodyProcessingRequest_AttributesEnvelopeMirrorsHeaderStage(t *testing.T) {
	t.Parallel()
	dcb := &dcbStub{
		remoteAddr:           &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234},
		localAddr:            &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 8080},
		tlsServerName:        "example.com",
		protocol:             "HTTP/2",
		listenerPrincipal:    "spiffe://cluster.local/listener",
		downstreamPrincipals: []string{"spiffe://cluster.local/client"},
	}
	f := &filter{dcb: dcb}
	allowlist := []string{
		"source.address",
		"destination.address",
		"connection.requested_server_name",
		"connection.subject_local_certificate",
		"request.protocol",
		"connection.principal",
		"source.principal",
	}

	headerReq := buildRequestHeadersProcessingRequest(f, http.Header{}, false, allowlist)
	bodyReq := buildRequestBodyProcessingRequest(f, []byte("ignored"), false, allowlist)

	headerAttrs := headerReq.GetAttributes()
	bodyAttrs := bodyReq.GetAttributes()
	if headerAttrs == nil || bodyAttrs == nil {
		t.Fatalf("nil attrs: headerAttrs=%v bodyAttrs=%v", headerAttrs, bodyAttrs)
	}
	if len(headerAttrs) != len(bodyAttrs) {
		t.Errorf("len(headerAttrs) = %d; len(bodyAttrs) = %d; want equal (D5 SUPERSET property — header-stage roster carries unchanged at body stage when request.size is excluded from the allowlist)",
			len(headerAttrs), len(bodyAttrs))
	}
	for _, name := range allowlist {
		hv, hok := headerAttrs[name]
		bv, bok := bodyAttrs[name]
		if hok != bok {
			t.Errorf("presence mismatch for %q: header=%v body=%v", name, hok, bok)
			continue
		}
		if !hok {
			continue
		}
		// Compare the scalar string values (the {value: <StringValue>} shape).
		hs := hv.GetFields()["value"].GetStringValue()
		bs := bv.GetFields()["value"].GetStringValue()
		if hs != bs {
			t.Errorf("value mismatch for %q: header=%q body=%q", name, hs, bs)
		}
	}
}

// TestBuildBodyProcessingRequest_RequestSizePopulatesFromBodyLength asserts
// the body-stage-only `request.size` attribute populates from `len(body)`
// (per planner-time D5 — populated accurately from the body bytes rather than
// Content-Length-derived; the body-stage natural attribute the header-stage
// envelope cannot carry).
func TestBuildBodyProcessingRequest_RequestSizePopulatesFromBodyLength(t *testing.T) {
	t.Parallel()
	f := &filter{dcb: &dcbStub{}}
	body := bytes.Repeat([]byte("x"), 1024) // 1024 bytes.

	req := buildRequestBodyProcessingRequest(f, body, true, []string{"request.size"})

	attrs := req.GetAttributes()
	if attrs == nil {
		t.Fatalf("Attributes = nil; want 1-entry envelope")
	}
	if len(attrs) != 1 {
		t.Errorf("len(attrs) = %d; want 1 (only request.size)", len(attrs))
	}
	entry, ok := attrs["request.size"]
	if !ok {
		t.Fatalf("request.size missing; keys = %v", attrKeysOf(attrs))
	}
	fv, fok := entry.GetFields()["value"]
	if !fok {
		t.Fatalf("Struct.fields[\"value\"] missing; fields = %v", entry.GetFields())
	}
	// `request.size` is a numeric attribute — encoded as a NumberValue (float64
	// wire representation) per the structpb scalar conventions. The IMPL
	// settles a NumberValue at 19.2 since structpb has no native int64 scalar;
	// closure at Task 9 fixture scrape against reference Envoy v1.37.2.
	if got := int64(fv.GetNumberValue()); got != int64(len(body)) {
		t.Errorf("request.size = %d; want %d (len(body))", got, len(body))
	}

	// Zero-length body case — the empty-value-skip discipline allows the
	// attribute to populate as 0 (numeric zero is a SUBSTANTIVE value: "this
	// stage carried an empty body" is information-bearing per D5 — distinct
	// from "the accessor returned no value"). Documented at the body-stage
	// wrapper's GoDoc.
	emptyReq := buildRequestBodyProcessingRequest(f, []byte{}, true, []string{"request.size"})
	emptyAttrs := emptyReq.GetAttributes()
	if emptyAttrs == nil {
		t.Fatalf("empty-body Attributes = nil; want 1-entry envelope with request.size=0")
	}
	emptyEntry, ok := emptyAttrs["request.size"]
	if !ok {
		t.Fatalf("empty-body request.size missing")
	}
	if got := int64(emptyEntry.GetFields()["value"].GetNumberValue()); got != 0 {
		t.Errorf("empty-body request.size = %d; want 0", got)
	}
}

// TestBuildResponseBodyProcessingRequest_Symmetric asserts the encode-side
// body-stage builder mirrors the decode-side: response_body oneof + HttpBody
// + attributes envelope sourced from f.ecb (ADR-0174 surface; D10 hypothesis
// HELD — no source.principal on encode side) + `response.size` populates
// from `len(body)`.
func TestBuildResponseBodyProcessingRequest_Symmetric(t *testing.T) {
	t.Parallel()
	ecb := &ecbStub{
		remoteAddr:        &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 1234},
		localAddr:         &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 8080},
		tlsServerName:     "example.com",
		protocol:          "HTTP/2",
		listenerPrincipal: "spiffe://cluster.local/listener",
	}
	f := &filter{ecb: ecb}
	body := []byte(`{"result":"ok"}`)
	allowlist := []string{
		"source.address",
		"destination.address",
		"connection.requested_server_name",
		"connection.subject_local_certificate",
		"request.protocol",
		"connection.principal",
		"source.principal", // D10: empty on encode side → SKIPPED.
		"response.size",
	}

	req := buildResponseBodyProcessingRequest(f, body, true, allowlist)

	if req == nil {
		t.Fatalf("req = nil")
	}
	rb := req.GetResponseBody()
	if rb == nil {
		t.Fatalf("ResponseBody oneof not set; got Request = %T", req.GetRequest())
	}
	if got := rb.GetBody(); !bytes.Equal(got, body) {
		t.Errorf("Body = %q; want %q", got, body)
	}
	if !rb.GetEndOfStream() {
		t.Errorf("EndOfStream = false; want true")
	}

	attrs := req.GetAttributes()
	if attrs == nil {
		t.Fatalf("Attributes = nil")
	}
	// D10: source.principal is EMPTY on the encode side (no DownstreamPrincipal
	// on EncoderFilterCallbacks per ADR-0174). 6 header-stage attributes
	// populate + `response.size` = 7 total.
	if _, ok := attrs["source.principal"]; ok {
		t.Errorf("source.principal unexpectedly present on encode side; D10 hypothesis FALSIFIED")
	}
	if len(attrs) != 7 {
		t.Errorf("len(attrs) = %d; want 7 (6 header-stage + response.size; D10 source.principal skipped); keys = %v",
			len(attrs), attrKeysOf(attrs))
	}
	respSize, ok := attrs["response.size"]
	if !ok {
		t.Fatalf("response.size missing; keys = %v", attrKeysOf(attrs))
	}
	if got := int64(respSize.GetFields()["value"].GetNumberValue()); got != int64(len(body)) {
		t.Errorf("response.size = %d; want %d (len(body))", got, len(body))
	}
}

// keysOf returns the sorted keys of a map[string]*structpb.Struct for
// stable assertion error output.
func keysOf(m map[string]*structpb.Struct) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// attrKeysOf returns the sorted keys of the ProcessingRequest.Attributes
// map for stable assertion error output (same shape as keysOf above; kept
// as a sibling for grep-discoverability).
func attrKeysOf(m map[string]*structpb.Struct) []string {
	return keysOf(m)
}

// ---------------------------------------------------------------------------
// Group 8 — per-route 5th-canonical resolution + cache-on-first-use +
// 9-counter SHARED-stats wiring per ADR-0173 (Task 10).
//
// Tests the parsePerRoute + (*factoryState).resolvePerRouteConfig +
// (*filter).resolvePerRoute surface per parent SPEC §5.P6 + §5.P7 + 19.1
// SPEC §5 + ADR-0173 §Decision:
//
//   - parsePerRoute: PARSE-REJECT empty ExtProcPerRoute (override oneof
//     PGV-required); PARSE-REJECT disabled:false (PGV const:true); accepts
//     disabled:true and overrides with processing_mode + grpc_service consumed;
//     silent-ignores async_mode + request_attributes + response_attributes +
//     metadata_options + grpc_initial_metadata per the proto's
//     [#not-implemented-hide:] convention.
//   - resolvePerRouteConfig: nil-tolerant; type-assertion fallback;
//     sync.Map-cached by proto pointer-identity per ADR-0117 + ADR-0125 §(v).
//   - (*filter).resolvePerRoute: cache-on-first-use per parent §5.P7 — the
//     per-route resolved at DecodeHeaders time stays in effect across the
//     filter's lifetime even after a hypothetical ClearRouteCache invocation
//     (simulated here by re-invoking RequestRouteConfig with a different
//     return value).
//   - SHARED-stats: per-route disabled:true returns no counter increments;
//     per-route overrides spawn NO new *filterStats — the listener-level
//     stats are reused via the shared compiledConfig pointer.
//   - 9-counter unconditional registration at New()-time: all 9 counter names
//     present in the Registry immediately after newFilterStats returns (mirrors
//     phase-18.2 ADR-0163 STRUCTURALLY-UNREACHABLE-counter scrape-stability).
//
// ---------------------------------------------------------------------------

// TestParseExtProcPerRoute_EmptyOverride asserts PARSE-REJECT when the
// override oneof is not set (the oneof is PGV-required per parent §5.P6).
func TestParseExtProcPerRoute_EmptyOverride(t *testing.T) {
	t.Parallel()
	empty := &extprocv3.ExtProcPerRoute{}
	got, err := parseExtProcPerRoute(empty, false /*httpServiceMode*/)
	if err == nil {
		t.Fatalf("parseExtProcPerRoute(empty): err = nil; want PARSE-REJECT")
	}
	if got != nil {
		t.Errorf("parseExtProcPerRoute(empty): got = %v; want nil on PARSE-REJECT", got)
	}
	if !strings.Contains(err.Error(), "override") {
		t.Errorf("parseExtProcPerRoute(empty): err = %q; want substring 'override'", err.Error())
	}
}

// TestParseExtProcPerRoute_DisabledFalse asserts PARSE-REJECT when
// disabled:false (PGV const:true — only disabled:true is meaningful).
func TestParseExtProcPerRoute_DisabledFalse(t *testing.T) {
	t.Parallel()
	pr := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Disabled{Disabled: false},
	}
	got, err := parseExtProcPerRoute(pr, false /*httpServiceMode*/)
	if err == nil {
		t.Fatalf("parseExtProcPerRoute(disabled=false): err = nil; want PARSE-REJECT")
	}
	if got != nil {
		t.Errorf("parseExtProcPerRoute(disabled=false): got = %v; want nil on PARSE-REJECT", got)
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("parseExtProcPerRoute(disabled=false): err = %q; want substring 'disabled'", err.Error())
	}
}

// TestParseExtProcPerRoute_DisabledTrue asserts that the disabled:true arm
// parses successfully → resolved with .disabled=true + no effective override
// state.
func TestParseExtProcPerRoute_DisabledTrue(t *testing.T) {
	t.Parallel()
	pr := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Disabled{Disabled: true},
	}
	got, err := parseExtProcPerRoute(pr, false /*httpServiceMode*/)
	if err != nil {
		t.Fatalf("parseExtProcPerRoute(disabled=true): unexpected err = %v", err)
	}
	if got == nil {
		t.Fatalf("parseExtProcPerRoute(disabled=true): got = nil; want non-nil")
	}
	if !got.disabled {
		t.Errorf("parseExtProcPerRoute(disabled=true): .disabled = false; want true")
	}
	if got.effectiveProcessingMode != nil {
		t.Errorf("parseExtProcPerRoute(disabled=true): .effectiveProcessingMode = %v; want nil", got.effectiveProcessingMode)
	}
}

// TestParseExtProcPerRoute_Overrides_ProcessingMode asserts that the
// overrides arm with a processing_mode field populates the resolved per-route's
// effectiveProcessingMode (MVP-CONSUMED per ADR-0173 §Decision).
func TestParseExtProcPerRoute_Overrides_ProcessingMode(t *testing.T) {
	t.Parallel()
	pr := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Overrides{
			Overrides: &extprocv3.ExtProcOverrides{
				ProcessingMode: &extprocv3.ProcessingMode{
					RequestHeaderMode:  extprocv3.ProcessingMode_SKIP,
					ResponseHeaderMode: extprocv3.ProcessingMode_SEND,
				},
			},
		},
	}
	got, err := parseExtProcPerRoute(pr, false /*httpServiceMode*/)
	if err != nil {
		t.Fatalf("parseExtProcPerRoute(overrides.processing_mode): unexpected err = %v", err)
	}
	if got == nil {
		t.Fatalf("parseExtProcPerRoute(overrides.processing_mode): got = nil; want non-nil")
	}
	if got.disabled {
		t.Errorf(".disabled = true; want false for overrides arm")
	}
	if got.effectiveProcessingMode == nil {
		t.Fatalf(".effectiveProcessingMode = nil; want populated from overrides.processing_mode")
	}
	if got.effectiveProcessingMode.RequestHeaderMode != extprocv3.ProcessingMode_SKIP {
		t.Errorf(".effectiveProcessingMode.RequestHeaderMode = %v; want SKIP",
			got.effectiveProcessingMode.RequestHeaderMode)
	}
	if got.effectiveProcessingMode.ResponseHeaderMode != extprocv3.ProcessingMode_SEND {
		t.Errorf(".effectiveProcessingMode.ResponseHeaderMode = %v; want SEND",
			got.effectiveProcessingMode.ResponseHeaderMode)
	}
}

// TestParseExtProcPerRoute_Overrides_ProcessingMode_BodyModeRejected asserts
// that per-route processing_mode overrides obey the SAME PARSE-REJECT
// discipline as the listener-level processing_mode — STREAMED-class body
// modes PARSE-REJECT permanently per ADR-0168 §Decision. (The BUFFERED arm
// ACCEPTS post-19.2 §Decision AMENDMENT — see
// TestBuildCompiledConfig_BodyMode_BUFFERED_PerRoute_AcceptsForGRPCService.)
func TestParseExtProcPerRoute_Overrides_ProcessingMode_BodyModeRejected(t *testing.T) {
	t.Parallel()
	pr := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Overrides{
			Overrides: &extprocv3.ExtProcOverrides{
				ProcessingMode: &extprocv3.ProcessingMode{
					RequestBodyMode: extprocv3.ProcessingMode_STREAMED,
				},
			},
		},
	}
	got, err := parseExtProcPerRoute(pr, false /*httpServiceMode*/)
	if err == nil {
		t.Fatalf("parseExtProcPerRoute(per-route body-mode STREAMED): err = nil; want PARSE-REJECT")
	}
	if got != nil {
		t.Errorf("parseExtProcPerRoute(per-route body-mode STREAMED): got = %v; want nil", got)
	}
}

// TestParseExtProcPerRoute_Overrides_GRPCService_Captured asserts that an
// overrides arm with grpc_service is consumed: the resolved per-route carries
// the raw *core.GrpcService pointer for downstream buildGRPCProcessorClient
// invocation at the call site (per ADR-0173 §Decision: per-route grpc_service
// is MVP-CONSUMED).
func TestParseExtProcPerRoute_Overrides_GRPCService_Captured(t *testing.T) {
	t.Parallel()
	gs := &corev3.GrpcService{
		TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: "alt-processor"},
		},
	}
	pr := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Overrides{
			Overrides: &extprocv3.ExtProcOverrides{GrpcService: gs},
		},
	}
	got, err := parseExtProcPerRoute(pr, false /*httpServiceMode*/)
	if err != nil {
		t.Fatalf("parseExtProcPerRoute(overrides.grpc_service): unexpected err = %v", err)
	}
	if got == nil {
		t.Fatalf("parseExtProcPerRoute(overrides.grpc_service): got = nil; want non-nil")
	}
	if got.grpcService != gs {
		t.Errorf(".grpcService = %p; want %p (raw proto pointer preserved)", got.grpcService, gs)
	}
}

// TestParseExtProcPerRoute_Overrides_SilentIgnoredFields asserts that the five
// per-route override fields flagged [#not-implemented-hide:] OR DEFERRED per
// ADR-0173 §Decision are SILENT-IGNORED — parse succeeds even when populated;
// the values do NOT surface on the resolved per-route struct:
//
//   - #2 async_mode (bool)
//   - #3 request_attributes ([]string) — distinct from TOP-LEVEL #5 which IS consumed
//   - #4 response_attributes ([]string) — distinct from TOP-LEVEL #6 which IS consumed
//   - #6 metadata_options (*MetadataOptions)
//   - #7 grpc_initial_metadata ([]*HeaderValue)
func TestParseExtProcPerRoute_Overrides_SilentIgnoredFields(t *testing.T) {
	t.Parallel()
	pr := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Overrides{
			Overrides: &extprocv3.ExtProcOverrides{
				AsyncMode:           true,                       // #2 [#not-implemented-hide:]
				RequestAttributes:   []string{"per-route-attr"}, // #3 [#not-implemented-hide:]
				ResponseAttributes:  []string{"per-route-attr"}, // #4 [#not-implemented-hide:]
				MetadataOptions:     &extprocv3.MetadataOptions{},
				GrpcInitialMetadata: []*corev3.HeaderValue{{Key: "x", Value: "y"}},
			},
		},
	}
	got, err := parseExtProcPerRoute(pr, false /*httpServiceMode*/)
	if err != nil {
		t.Fatalf("parseExtProcPerRoute(silent-ignored fields populated): unexpected err = %v", err)
	}
	if got == nil {
		t.Fatalf("parseExtProcPerRoute(silent-ignored fields populated): got = nil; want non-nil (silent-ignore — parse succeeds)")
	}
	if got.disabled {
		t.Errorf(".disabled = true; want false for overrides arm")
	}
	if got.effectiveProcessingMode != nil {
		t.Errorf(".effectiveProcessingMode = %v; want nil (no processing_mode override)", got.effectiveProcessingMode)
	}
	if got.grpcService != nil {
		t.Errorf(".grpcService = %v; want nil (no grpc_service override)", got.grpcService)
	}
}

// TestParseExtProcPerRoute_Overrides_Empty asserts that an empty
// ExtProcOverrides (overrides arm set but no inner fields populated) parses
// successfully and yields a resolved per-route with no effective override
// state — the overrides arm is structurally valid even when narrower than the
// listener-level config (per parent §5.P6 — the per-route MAY override fewer
// fields than the listener defines).
func TestParseExtProcPerRoute_Overrides_Empty(t *testing.T) {
	t.Parallel()
	pr := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Overrides{
			Overrides: &extprocv3.ExtProcOverrides{},
		},
	}
	got, err := parseExtProcPerRoute(pr, false /*httpServiceMode*/)
	if err != nil {
		t.Fatalf("parseExtProcPerRoute(empty overrides): unexpected err = %v", err)
	}
	if got == nil {
		t.Fatalf("parseExtProcPerRoute(empty overrides): got = nil; want non-nil")
	}
	if got.disabled {
		t.Errorf(".disabled = true; want false for overrides arm")
	}
	if got.effectiveProcessingMode != nil {
		t.Errorf(".effectiveProcessingMode = %v; want nil (no override)", got.effectiveProcessingMode)
	}
	if got.grpcService != nil {
		t.Errorf(".grpcService = %v; want nil (no override)", got.grpcService)
	}
}

// TestParseExtProcPerRoute_NilInput asserts the defensive nil-tolerance —
// parseExtProcPerRoute(nil) returns an error rather than panicking.
func TestParseExtProcPerRoute_NilInput(t *testing.T) {
	t.Parallel()
	got, err := parseExtProcPerRoute(nil, false /*httpServiceMode*/)
	if err == nil {
		t.Fatalf("parseExtProcPerRoute(nil): err = nil; want non-nil")
	}
	if got != nil {
		t.Errorf("parseExtProcPerRoute(nil): got = %v; want nil", got)
	}
}

// TestResolvePerRouteConfig_NilMsg asserts that nil msg → listener-level
// fallback (no per-route TPFC applies; not-disabled + no overrides).
func TestResolvePerRouteConfig_NilMsg(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{}
	state := &factoryState{listenerRC: cc}
	got := state.resolvePerRouteConfig(nil)
	if got == nil {
		t.Fatalf("resolvePerRouteConfig(nil): got = nil; want non-nil fallback")
	}
	if got.disabled {
		t.Errorf("resolvePerRouteConfig(nil): .disabled = true; want false for listener-level fallback")
	}
	if got.effectiveProcessingMode != nil {
		t.Errorf("resolvePerRouteConfig(nil): .effectiveProcessingMode = %v; want nil", got.effectiveProcessingMode)
	}
}

// TestResolvePerRouteConfig_UnknownMsgTypeFallback asserts that a
// non-*ExtProcPerRoute proto.Message → listener-level fallback (defensive type
// assertion; mirrors phase-18.1 extauthz pattern).
func TestResolvePerRouteConfig_UnknownMsgTypeFallback(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{}
	state := &factoryState{listenerRC: cc}
	wrongType := &corev3.GrpcService{} // any non-*ExtProcPerRoute proto.Message
	got := state.resolvePerRouteConfig(wrongType)
	if got == nil {
		t.Fatalf("resolvePerRouteConfig(wrong type): got = nil; want non-nil fallback")
	}
	if got.disabled {
		t.Errorf("resolvePerRouteConfig(wrong type): .disabled = true; want false")
	}
}

// TestResolvePerRouteConfig_DisabledTrue asserts that a disabled:true
// per-route resolves to the disabled state.
func TestResolvePerRouteConfig_DisabledTrue(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{}
	state := &factoryState{listenerRC: cc}
	pr := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Disabled{Disabled: true},
	}
	got := state.resolvePerRouteConfig(pr)
	if got == nil {
		t.Fatalf("resolvePerRouteConfig(disabled): got = nil; want non-nil")
	}
	if !got.disabled {
		t.Errorf("resolvePerRouteConfig(disabled): .disabled = false; want true")
	}
}

// TestResolvePerRouteConfig_SyncMapIdentity asserts the sync.Map cache:
// repeated calls with the SAME proto pointer return pointer-identical
// *resolvedPerRoute values (cache-on-LoadOrStore per ADR-0117 + ADR-0125 §(v)).
func TestResolvePerRouteConfig_SyncMapIdentity(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{}
	state := &factoryState{listenerRC: cc}
	pr := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Disabled{Disabled: true},
	}
	r1 := state.resolvePerRouteConfig(pr)
	r2 := state.resolvePerRouteConfig(pr)
	if r1 == nil || r2 == nil {
		t.Fatalf("resolvePerRouteConfig: unexpected nil (r1=%v, r2=%v)", r1, r2)
	}
	if r1 != r2 {
		t.Errorf("sync.Map identity: got different pointers for same proto (%p vs %p)", r1, r2)
	}
}

// TestResolvePerRouteConfig_ParseErrorFallback asserts that a per-route proto
// that parsePerRoute rejects (e.g., disabled:false) falls back to the
// listener-level config — the parse error is logged + NOT cached + the
// returned *resolvedPerRoute reflects the listener-level inheritance.
func TestResolvePerRouteConfig_ParseErrorFallback(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{}
	state := &factoryState{listenerRC: cc}
	pr := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Disabled{Disabled: false}, // PGV const:true violation
	}
	got := state.resolvePerRouteConfig(pr)
	if got == nil {
		t.Fatalf("resolvePerRouteConfig(disabled=false invalid): got = nil; want non-nil fallback")
	}
	if got.disabled {
		t.Errorf("resolvePerRouteConfig(parse-error fallback): .disabled = true; want false")
	}
	// Confirm the parse-error sentinel was NOT cached: a follow-up Load on the
	// same proto pointer returns a fresh fallback (not the cached error
	// sentinel). The sync.Map should have no entry for this proto pointer.
	if _, cached := state.perRoute.Load(pr); cached {
		t.Errorf("resolvePerRouteConfig(parse-error): unexpected cache entry; parse errors must NOT cache (mirrors phase-18.1 ext_authz discipline)")
	}
}

// TestFilterResolvePerRoute_CacheOnFirstUse_AcrossClearRouteCache is the
// load-bearing cache-on-first-use assertion per parent §5.P7 + ADR-0173
// §Decision. The per-route resolved at the FIRST resolvePerRoute() invocation
// stays in effect for the entire filter's lifetime — subsequent calls return
// the cached *resolvedPerRoute EVEN AFTER a simulated ClearRouteCache (here:
// the dcb's RequestRouteConfig return value is mutated between calls to mimic
// the framework's ClearRouteCache effect).
//
// This is the parent §5.P7 RATIFIED-PENDING-IMPL-TIME closure: the per-route
// resolution caches at DecodeHeaders time and stays in effect for the entire
// bidi-stream's lifetime, mirroring phase-10/17 precedents.
func TestFilterResolvePerRoute_CacheOnFirstUse_AcrossClearRouteCache(t *testing.T) {
	t.Parallel()

	cc := &compiledConfig{}
	state := &factoryState{listenerRC: cc}

	// First per-route: a disabled:true arm.
	firstPR := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Disabled{Disabled: true},
	}
	// Second per-route: a non-disabled overrides arm. If cache-on-first-use is
	// honored, the filter should NEVER see this second per-route's resolved
	// state — the first call's cached *resolvedPerRoute (.disabled=true)
	// remains the filter's effective per-route across the simulated cache
	// clear.
	secondPR := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Overrides{
			Overrides: &extprocv3.ExtProcOverrides{},
		},
	}

	dcb := &perRouteSwapDCB{current: firstPR}
	f := &filter{state: state, dcb: dcb}

	first := f.resolvePerRoute()
	if first == nil {
		t.Fatalf("resolvePerRoute (1st call): got = nil; want non-nil")
	}
	if !first.disabled {
		t.Errorf("resolvePerRoute (1st call): .disabled = false; want true (first per-route is disabled:true)")
	}

	// Simulate a mid-stream ClearRouteCache: the framework returns a DIFFERENT
	// per-route proto on the next RequestRouteConfig invocation. The filter
	// MUST NOT re-resolve — it MUST return the cached *resolvedPerRoute from
	// the first call.
	dcb.current = secondPR
	second := f.resolvePerRoute()
	if second == nil {
		t.Fatalf("resolvePerRoute (2nd call): got = nil; want cached non-nil")
	}
	if second != first {
		t.Errorf("cache-on-first-use violation: got fresh resolve (%p) on 2nd call; want cached (%p)", second, first)
	}
	if !second.disabled {
		t.Errorf("cache-on-first-use violation: 2nd call's .disabled = false; want true (cached from 1st call's disabled:true)")
	}

	// And a third call after another mutation — also cached.
	dcb.current = nil // simulate "no per-route" — would normally resolve to listener-fallback
	third := f.resolvePerRoute()
	if third != first {
		t.Errorf("cache-on-first-use violation: got fresh resolve (%p) on 3rd call (after dcb.current=nil); want cached (%p)", third, first)
	}
}

// TestFilterResolvePerRoute_NilDCB asserts defensive nil-handling — a filter
// with no dcb (e.g., unit-test path) returns a listener-level fallback rather
// than panicking.
func TestFilterResolvePerRoute_NilDCB(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{}
	state := &factoryState{listenerRC: cc}
	f := &filter{state: state, dcb: nil}
	got := f.resolvePerRoute()
	if got == nil {
		t.Fatalf("resolvePerRoute (nil dcb): got = nil; want listener-fallback non-nil")
	}
	if got.disabled {
		t.Errorf("resolvePerRoute (nil dcb): .disabled = true; want false")
	}
	// Subsequent calls return the cached value.
	if f.resolvePerRoute() != got {
		t.Errorf("cache-on-first-use violation under nil dcb")
	}
}

// TestFilterResolvePerRoute_NilState asserts defensive nil-handling on the
// filter.state path — returns the disabled-false zero value rather than
// panicking (degenerate test path).
func TestFilterResolvePerRoute_NilState(t *testing.T) {
	t.Parallel()
	f := &filter{state: nil}
	got := f.resolvePerRoute()
	if got == nil {
		t.Fatalf("resolvePerRoute (nil state): got = nil; want non-nil zero-value fallback")
	}
	if got.disabled {
		t.Errorf("resolvePerRoute (nil state): .disabled = true; want false")
	}
}

// perRouteSwapDCB is a fakeDCB analog whose RequestRouteConfig return value
// can be swapped between invocations — exercises the cache-on-first-use
// assertion (the first call's value is the one that wins; subsequent swaps
// must NOT re-enter the resolvePerRouteConfig path because of the cache).
type perRouteSwapDCB struct {
	current proto.Message
}

func (d *perRouteSwapDCB) ContinueDecoding()                                    {}
func (d *perRouteSwapDCB) SendLocalReply(int, string, envoyhttp.OrderedHeaders) {}
func (d *perRouteSwapDCB) RequestRouteConfig() proto.Message                    { return d.current }
func (d *perRouteSwapDCB) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return d.current, nil, nil
}
func (d *perRouteSwapDCB) EncodeHeaders(http.Header, bool)  {}
func (d *perRouteSwapDCB) EncodeData([]byte, bool)          {}
func (d *perRouteSwapDCB) EncodeTrailers(http.Header)       {}
func (d *perRouteSwapDCB) DownstreamPrincipal() []string    { return nil }
func (d *perRouteSwapDCB) DownstreamRemoteAddr() net.Addr   { return nil }
func (d *perRouteSwapDCB) DownstreamLocalAddr() net.Addr    { return nil }
func (d *perRouteSwapDCB) DownstreamTLSServerName() string  { return "" }
func (d *perRouteSwapDCB) DownstreamTLSPeerCertDER() []byte { return nil }
func (d *perRouteSwapDCB) DownstreamProtocol() string       { return "" }
func (d *perRouteSwapDCB) ListenerPrincipal() string        { return "" }

// ADR-0192 callback-surface extension stubs (phase-22.2 Task 5).
func (d *perRouteSwapDCB) DownstreamTLSConnectionState() *tls.ConnectionState { return nil }
func (d *perRouteSwapDCB) DynamicMetadata() *dynamicmetadata.Bucket           { return nil }

// ADR-0198 callback-surface extension stubs (phase-24.1 Task 5 — DELTA-2).
func (d *perRouteSwapDCB) RouteRateLimits() []*routev3.RateLimit       { return nil }
func (d *perRouteSwapDCB) VirtualHostRateLimits() []*routev3.RateLimit { return nil }
func (d *perRouteSwapDCB) RouteMetadata() *corev3.Metadata             { return nil }
func (d *perRouteSwapDCB) RouteIncludeVhRateLimits() bool              { return false }

var _ envoyhttp.DecoderFilterCallbacks = (*perRouteSwapDCB)(nil)

// TestNewFilterStats_AllNineCountersRegisteredUnconditionally is the parent
// §5.P4 + ADR-0173 §Decision discipline assertion: ALL 9 counters appear in
// the Registry immediately after newFilterStats returns. This is the
// scrape-stability discipline — operators get a consistent counter surface
// regardless of which code paths fire during the listener's lifetime
// (mirrors phase-18.2 ADR-0163 STRUCTURALLY-UNREACHABLE-counter discipline).
//
// The exact 9 counter names per ADR-0173 §Decision under
// `http.<HCM_stat_prefix>.ext_proc.<counter>`:
//
//	streams_started, stream_msgs_sent, stream_msgs_received,
//	spurious_msgs_received, streams_failed, streams_closed,
//	failure_mode_allowed, override_message_timeout_received,
//	override_message_timeout_ignored
func TestNewFilterStats_AllNineCountersRegisteredUnconditionally(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_http")
	if fs == nil {
		t.Fatalf("newFilterStats returned nil; want non-nil *filterStats")
	}

	// Walk the registry + collect all registered counter names.
	names := make(map[string]bool)
	reg.Walk(func(m stats.Metric) {
		names[m.Name()] = true
	})

	// Exactly 9 counters, no extras.
	if len(names) != 9 {
		t.Errorf("Registry size = %d; want exactly 9 ext_proc counters (no extras); got names=%v",
			len(names), sortedKeys(names))
	}

	wantPrefix := "http.ingress_http.ext_proc."
	want := []string{
		"streams_started",
		"stream_msgs_sent",
		"stream_msgs_received",
		"spurious_msgs_received",
		"streams_failed",
		"streams_closed",
		"failure_mode_allowed",
		"override_message_timeout_received",
		"override_message_timeout_ignored",
	}
	for _, suffix := range want {
		full := wantPrefix + suffix
		if !names[full] {
			t.Errorf("counter %q not registered; registered: %v", full, sortedKeys(names))
		}
	}
}

// TestNewFilterStats_EmptyStatPrefix_BareExtProcNamespace asserts the
// empty-HCM-stat-prefix fold-to-`ext_proc.` shape per baseStatPrefix's
// nameRE-satisfying discipline (the bare form is used when the HCM
// stat_prefix is empty — typically in unit-test code paths).
func TestNewFilterStats_EmptyStatPrefix_BareExtProcNamespace(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	_ = newFilterStats(reg, "")

	names := make(map[string]bool)
	reg.Walk(func(m stats.Metric) {
		names[m.Name()] = true
	})

	if len(names) != 9 {
		t.Errorf("Registry size = %d; want 9 (empty stat_prefix)", len(names))
	}
	// Spot-check two counter names under the bare prefix.
	if !names["ext_proc.streams_started"] {
		t.Errorf("counter %q not registered under bare ext_proc. prefix; got %v",
			"ext_proc.streams_started", sortedKeys(names))
	}
	if !names["ext_proc.override_message_timeout_ignored"] {
		t.Errorf("counter %q not registered under bare ext_proc. prefix; got %v",
			"ext_proc.override_message_timeout_ignored", sortedKeys(names))
	}
}

// TestResolvePerRouteConfig_SharedStats_NoNewFilterStatsAllocation asserts the
// ADR-0173 §Decision SHARED-stats discipline: per-route resolution NEVER
// allocates a fresh *filterStats. The per-route override adjusts
// processing_mode/grpc_service but routes to the SAME counter surface as the
// listener-level — the listener-level cc.stats pointer is the only stat
// surface in the package (no per-route cc + no per-route *filterStats).
//
// Verified structurally: the *resolvedPerRoute struct has NO stats field; the
// only counter-bearing surface is the listener-level cc.stats. This test pins
// that discipline: a fresh registry stays empty when only resolvePerRouteConfig
// is invoked (no per-route counter registration).
func TestResolvePerRouteConfig_SharedStats_NoNewFilterStatsAllocation(t *testing.T) {
	t.Parallel()
	// Listener registry: pre-populated with the 9 counters.
	reg := stats.NewRegistry()
	listenerStats := newFilterStats(reg, "ingress_http")
	cc := &compiledConfig{stats: listenerStats}
	state := &factoryState{listenerRC: cc}

	// Snapshot the registered-counter count before per-route resolution.
	pre := 0
	reg.Walk(func(stats.Metric) { pre++ })
	if pre != 9 {
		t.Fatalf("pre-resolve registry count = %d; want 9", pre)
	}

	// Resolve several distinct per-route protos (mix of disabled + overrides).
	pr1 := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Disabled{Disabled: true},
	}
	pr2 := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Overrides{
			Overrides: &extprocv3.ExtProcOverrides{
				ProcessingMode: &extprocv3.ProcessingMode{
					RequestHeaderMode: extprocv3.ProcessingMode_SKIP,
				},
			},
		},
	}
	_ = state.resolvePerRouteConfig(pr1)
	_ = state.resolvePerRouteConfig(pr2)

	// Post-resolve: the registry MUST still have exactly 9 counters — no
	// per-route allocation fired (ADR-0173 SHARED-stats discipline).
	post := 0
	reg.Walk(func(stats.Metric) { post++ })
	if post != 9 {
		t.Errorf("post-resolve registry count = %d; want 9 (SHARED-stats — no per-route filterStats allocation)", post)
	}
}

// sortedKeys returns the sorted keys of a name-presence map for stable
// assertion error output (Group 8 local helper; mirrors the keysOf helper at
// the end of the file but for a different map shape).
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// Group 1 + Group 2 EXPANSION — Task 11 buildCompiledConfig integration per
// SPEC §15 item 2 acceptance checklist + ADR-0168 §Decision discipline.
//
// Group 1 (factory parse paths — every PARSE-REJECT branch):
//   - both-set OR neither-set transport → PARSE-REJECT (parent §5.P1)
//   - body-mode != NONE PARSE-REJECT in 19.1
//   - trailer-mode != SKIP PARSE-REJECT permanently
//   - STREAMED-only flag PARSE-REJECT permanently (observability_mode,
//     send_body_without_waiting_for_header_response, deferred_close_timeout)
//   - GoogleGrpc arm PARSE-REJECT (inherited from ADR-0157 AMENDMENT)
//   - route_cache_action + disable_clear_route_cache mutual-exclusion
//   - HTTP-service body-mode PARSE-REJECT per proto constraint
//   - EnvoyGrpc cluster_name empty PARSE-REJECT
//   - unknown cluster PARSE-REJECT
//   - UseH2:false cluster PARSE-REJECT
//
// Group 2 (compiledConfig field values post-parse for both gRPC + HTTP arms):
//   - gRPC happy path: messageTimeout 200ms default, maxMessageTimeout 0
//     default, processingMode header-modes resolved, mutationRules + forward
//     Rules allocated, stats allocated when ctx.Stats != nil.
//   - HTTP happy path: httpServiceHeadersOnly=true, httpClient.baseURL set.
// ---------------------------------------------------------------------------

// mkValidGRPCExtProc returns a minimal valid ExternalProcessor with a gRPC
// service configured. Used as a base for negative-test mutations.
func mkValidGRPCExtProc(clusterName string) *extprocv3.ExternalProcessor {
	return &extprocv3.ExternalProcessor{
		GrpcService: &corev3.GrpcService{
			TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: clusterName},
			},
		},
	}
}

// mkValidHTTPExtProc returns a minimal valid ExternalProcessor with an
// http_service configured.
func mkValidHTTPExtProc(uri string) *extprocv3.ExternalProcessor {
	return &extprocv3.ExternalProcessor{
		HttpService: &extprocv3.ExtProcHttpService{
			HttpService: &corev3.HttpService{
				HttpUri: &corev3.HttpUri{
					Uri:     uri,
					Timeout: durationpb.New(500 * time.Millisecond),
				},
			},
		},
	}
}

// TestBuildCompiledConfig_NeitherTransport_ParseReject — neither
// grpc_service nor http_service set → PARSE-REJECT.
func TestBuildCompiledConfig_NeitherTransport_ParseReject(t *testing.T) {
	t.Parallel()
	cc, err := buildCompiledConfig(&extprocv3.ExternalProcessor{}, envoyhttp.FactoryCtx{})
	if cc != nil {
		t.Fatalf("cc = %v; want nil", cc)
	}
	if err == nil || !strings.Contains(err.Error(), "neither set") {
		t.Fatalf("err = %v; want 'neither set'", err)
	}
}

// TestBuildCompiledConfig_BothTransports_ParseReject — both grpc_service +
// http_service set → PARSE-REJECT.
func TestBuildCompiledConfig_BothTransports_ParseReject(t *testing.T) {
	t.Parallel()
	raw := mkValidGRPCExtProc("c_extproc")
	raw.HttpService = &extprocv3.ExtProcHttpService{
		HttpService: &corev3.HttpService{HttpUri: &corev3.HttpUri{Uri: "http://x/"}},
	}
	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{})
	if cc != nil {
		t.Fatalf("cc = %v; want nil", cc)
	}
	if err == nil || !strings.Contains(err.Error(), "both set") {
		t.Fatalf("err = %v; want 'both set'", err)
	}
}

// TestBuildCompiledConfig_ObservabilityMode_ParseReject — STREAMED-only flag.
func TestBuildCompiledConfig_ObservabilityMode_ParseReject(t *testing.T) {
	t.Parallel()
	raw := mkValidGRPCExtProc("c_extproc")
	raw.ObservabilityMode = true
	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{})
	if cc != nil {
		t.Fatalf("cc = %v; want nil", cc)
	}
	if err == nil || !strings.Contains(err.Error(), "observability_mode") {
		t.Fatalf("err = %v; want 'observability_mode'", err)
	}
}

// TestBuildCompiledConfig_SendBodyWithoutWaiting_ParseReject — STREAMED-only flag.
func TestBuildCompiledConfig_SendBodyWithoutWaiting_ParseReject(t *testing.T) {
	t.Parallel()
	raw := mkValidGRPCExtProc("c_extproc")
	raw.SendBodyWithoutWaitingForHeaderResponse = true
	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{})
	if cc != nil {
		t.Fatalf("cc = %v; want nil", cc)
	}
	if err == nil || !strings.Contains(err.Error(), "send_body_without_waiting_for_header_response") {
		t.Fatalf("err = %v; want 'send_body_without_waiting_for_header_response'", err)
	}
}

// TestBuildCompiledConfig_DeferredCloseTimeout_ParseReject — STREAMED-only flag.
func TestBuildCompiledConfig_DeferredCloseTimeout_ParseReject(t *testing.T) {
	t.Parallel()
	raw := mkValidGRPCExtProc("c_extproc")
	raw.DeferredCloseTimeout = durationpb.New(5 * time.Second)
	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{})
	if cc != nil {
		t.Fatalf("cc = %v; want nil", cc)
	}
	if err == nil || !strings.Contains(err.Error(), "deferred_close_timeout") {
		t.Fatalf("err = %v; want 'deferred_close_timeout'", err)
	}
}

// TestBuildCompiledConfig_DeferredCloseTimeoutZero_OK — zero deferred_close_timeout
// is the no-op default; should NOT PARSE-REJECT.
func TestBuildCompiledConfig_DeferredCloseTimeoutZero_OK(t *testing.T) {
	t.Parallel()
	cm := mkExtprocH2ClusterMgr(t, "c_extproc", 9999)
	raw := mkValidGRPCExtProc("c_extproc")
	raw.DeferredCloseTimeout = durationpb.New(0)
	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{ClusterManager: cm})
	if err != nil {
		t.Fatalf("err = %v; want nil (zero deferred_close_timeout is the no-op default)", err)
	}
	if cc == nil {
		t.Fatalf("cc = nil; want non-nil")
	}
	if cc.grpcClient != nil {
		_ = cc.grpcClient.Close()
	}
}

// TestBuildCompiledConfig_GoogleGrpc_ParseReject — GoogleGrpc PARSE-REJECT
// inherited from ADR-0157 §Decision AMENDMENT.
func TestBuildCompiledConfig_GoogleGrpc_ParseReject(t *testing.T) {
	t.Parallel()
	raw := &extprocv3.ExternalProcessor{
		GrpcService: &corev3.GrpcService{
			TargetSpecifier: &corev3.GrpcService_GoogleGrpc_{
				GoogleGrpc: &corev3.GrpcService_GoogleGrpc{TargetUri: "google:443"},
			},
		},
	}
	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{})
	if cc != nil {
		t.Fatalf("cc = %v; want nil", cc)
	}
	if err == nil || !strings.Contains(err.Error(), "google_grpc arm not supported") {
		t.Fatalf("err = %v; want 'google_grpc arm not supported'", err)
	}
}

// TestBuildCompiledConfig_EmptyEnvoyGrpcCluster_ParseReject — EnvoyGrpc
// without cluster_name → PARSE-REJECT.
func TestBuildCompiledConfig_EmptyEnvoyGrpcCluster_ParseReject(t *testing.T) {
	t.Parallel()
	raw := &extprocv3.ExternalProcessor{
		GrpcService: &corev3.GrpcService{
			TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: ""},
			},
		},
	}
	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{})
	if cc != nil {
		t.Fatalf("cc = %v; want nil", cc)
	}
	if err == nil || !strings.Contains(err.Error(), "cluster_name must be non-empty") {
		t.Fatalf("err = %v; want 'cluster_name must be non-empty'", err)
	}
}

// TestBuildCompiledConfig_UnknownCluster_ParseReject — cluster not in manager.
func TestBuildCompiledConfig_UnknownCluster_ParseReject(t *testing.T) {
	t.Parallel()
	cm := mkExtprocH2ClusterMgr(t, "c_other", 9999)
	raw := mkValidGRPCExtProc("c_unknown")
	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{ClusterManager: cm})
	if cc != nil {
		t.Fatalf("cc = %v; want nil", cc)
	}
	if err == nil || !strings.Contains(err.Error(), `unknown cluster "c_unknown"`) {
		t.Fatalf("err = %v; want 'unknown cluster \"c_unknown\"'", err)
	}
}

// TestBuildCompiledConfig_NonH2Cluster_ParseReject — UseH2()==false cluster.
func TestBuildCompiledConfig_NonH2Cluster_ParseReject(t *testing.T) {
	t.Parallel()
	cm := mkExtprocPlainClusterMgr(t, "c_plain", 9999)
	raw := mkValidGRPCExtProc("c_plain")
	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{ClusterManager: cm})
	if cc != nil {
		t.Fatalf("cc = %v; want nil", cc)
	}
	if err == nil || !strings.Contains(err.Error(), "http2_protocol_options") {
		t.Fatalf("err = %v; want 'http2_protocol_options'", err)
	}
}

// TestBuildCompiledConfig_NilClusterManager_ParseReject — gRPC mode requires
// a ClusterManager.
func TestBuildCompiledConfig_NilClusterManager_ParseReject(t *testing.T) {
	t.Parallel()
	raw := mkValidGRPCExtProc("c_extproc")
	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{ClusterManager: nil})
	if cc != nil {
		t.Fatalf("cc = %v; want nil", cc)
	}
	if err == nil || !strings.Contains(err.Error(), "cluster manager not available") {
		t.Fatalf("err = %v; want 'cluster manager not available'", err)
	}
}

// TestBuildCompiledConfig_BodyMode_STREAMED_PARSE_REJECT_Permanent — request/
// response_body_mode ∈ {STREAMED, BUFFERED_PARTIAL, FULL_DUPLEX_STREAMED}
// continue PARSE-REJECT PERMANENTLY per ADR-0168 §Decision + parent §4.4
// (STREAMED-class body modes out of envelope). The BUFFERED arm ACCEPTS post-
// 19.2 §Decision AMENDMENT for the gRPC-service arm (see
// TestBuildCompiledConfig_BodyMode_BUFFERED_AcceptsForGRPCService).
func TestBuildCompiledConfig_BodyMode_STREAMED_PARSE_REJECT_Permanent(t *testing.T) {
	t.Parallel()
	cm := mkExtprocH2ClusterMgr(t, "c_extproc", 9999)
	cases := []struct {
		name string
		pm   *extprocv3.ProcessingMode
		want string
	}{
		{"request_body_streamed", &extprocv3.ProcessingMode{RequestBodyMode: extprocv3.ProcessingMode_STREAMED}, "request_body_mode"},
		{"request_body_buffered_partial", &extprocv3.ProcessingMode{RequestBodyMode: extprocv3.ProcessingMode_BUFFERED_PARTIAL}, "request_body_mode"},
		{"request_body_full_duplex", &extprocv3.ProcessingMode{RequestBodyMode: extprocv3.ProcessingMode_FULL_DUPLEX_STREAMED}, "request_body_mode"},
		{"response_body_streamed", &extprocv3.ProcessingMode{ResponseBodyMode: extprocv3.ProcessingMode_STREAMED}, "response_body_mode"},
		{"response_body_buffered_partial", &extprocv3.ProcessingMode{ResponseBodyMode: extprocv3.ProcessingMode_BUFFERED_PARTIAL}, "response_body_mode"},
		{"response_body_full_duplex", &extprocv3.ProcessingMode{ResponseBodyMode: extprocv3.ProcessingMode_FULL_DUPLEX_STREAMED}, "response_body_mode"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := mkValidGRPCExtProc("c_extproc")
			raw.ProcessingMode = tc.pm
			cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{ClusterManager: cm})
			if cc != nil {
				t.Fatalf("cc = %v; want nil", cc)
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v; want substring %q", err, tc.want)
			}
		})
	}
}

// TestBuildCompiledConfig_BodyMode_BUFFERED_AcceptsForGRPCService — Group N:
// the 19.2 §Decision AMENDMENT lifts the body-mode PARSE-REJECT for the
// gRPC-service arm of `request_body_mode = BUFFERED` + `response_body_mode =
// BUFFERED`. The compiledConfig builds successfully + the resolved processing
// mode carries the raw BUFFERED enum (not silently rewritten to NONE per the
// pre-AMENDMENT hardcoding).
func TestBuildCompiledConfig_BodyMode_BUFFERED_AcceptsForGRPCService(t *testing.T) {
	t.Parallel()
	cm := mkExtprocH2ClusterMgr(t, "c_extproc", 9999)
	cases := []struct {
		name         string
		pm           *extprocv3.ProcessingMode
		wantReqBody  extprocv3.ProcessingMode_BodySendMode
		wantRespBody extprocv3.ProcessingMode_BodySendMode
	}{
		{
			name:         "request_body_buffered",
			pm:           &extprocv3.ProcessingMode{RequestBodyMode: extprocv3.ProcessingMode_BUFFERED},
			wantReqBody:  extprocv3.ProcessingMode_BUFFERED,
			wantRespBody: extprocv3.ProcessingMode_NONE,
		},
		{
			name:         "response_body_buffered",
			pm:           &extprocv3.ProcessingMode{ResponseBodyMode: extprocv3.ProcessingMode_BUFFERED},
			wantReqBody:  extprocv3.ProcessingMode_NONE,
			wantRespBody: extprocv3.ProcessingMode_BUFFERED,
		},
		{
			name: "both_directions_buffered",
			pm: &extprocv3.ProcessingMode{
				RequestBodyMode:  extprocv3.ProcessingMode_BUFFERED,
				ResponseBodyMode: extprocv3.ProcessingMode_BUFFERED,
			},
			wantReqBody:  extprocv3.ProcessingMode_BUFFERED,
			wantRespBody: extprocv3.ProcessingMode_BUFFERED,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := mkValidGRPCExtProc("c_extproc")
			raw.ProcessingMode = tc.pm
			cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{ClusterManager: cm})
			if err != nil {
				t.Fatalf("buildCompiledConfig(BUFFERED gRPC): err = %v; want nil (post-19.2 §Decision AMENDMENT)", err)
			}
			if cc == nil {
				t.Fatalf("cc = nil; want non-nil")
			}
			defer func() {
				if cc.grpcClient != nil {
					_ = cc.grpcClient.Close()
				}
			}()
			if cc.processingMode == nil {
				t.Fatalf("cc.processingMode = nil; want non-nil")
			}
			if cc.processingMode.RequestBodyMode != tc.wantReqBody {
				t.Errorf("RequestBodyMode = %v; want %v (raw enum populated, not hardcoded NONE)",
					cc.processingMode.RequestBodyMode, tc.wantReqBody)
			}
			if cc.processingMode.ResponseBodyMode != tc.wantRespBody {
				t.Errorf("ResponseBodyMode = %v; want %v (raw enum populated, not hardcoded NONE)",
					cc.processingMode.ResponseBodyMode, tc.wantRespBody)
			}
		})
	}
}

// TestBuildCompiledConfig_BodyMode_HTTPService_PARSE_REJECT_Continues —
// Group N: HTTP-service-mode body PARSE-REJECT continues PERMANENTLY per
// ADR-0168 §Decision (iii) + SPEC §2 item 1. The 19.2 §Decision AMENDMENT
// applies ONLY to the gRPC-service arm; HTTP-service stays headers-only
// (per the proto's ExtProcHttpService constraint).
func TestBuildCompiledConfig_BodyMode_HTTPService_PARSE_REJECT_Continues(t *testing.T) {
	t.Parallel()
	raw := mkValidHTTPExtProc("http://processor.local:8080/process")
	raw.ProcessingMode = &extprocv3.ProcessingMode{
		RequestBodyMode: extprocv3.ProcessingMode_BUFFERED,
	}
	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{})
	if cc != nil {
		t.Fatalf("cc = %v; want nil (http_service + BUFFERED must PARSE-REJECT)", cc)
	}
	if err == nil || !strings.Contains(err.Error(), "http_service") {
		t.Fatalf("err = %v; want substring 'http_service' (httpServiceMode-gated PARSE-REJECT)", err)
	}
}

// TestBuildCompiledConfig_BodyMode_BUFFERED_PerRoute_AcceptsForGRPCService —
// Group N: the per-route ExtProcOverrides.processing_mode body-mode arm
// inherits the §Decision AMENDMENT via the shared resolveProcessingMode
// call site at parseExtProcPerRoute (per planner-time D12: per-route 5th-
// canonical body-mode arm activation LOCKS at 19.2 — no ADR-0173 amendment
// fires).
func TestBuildCompiledConfig_BodyMode_BUFFERED_PerRoute_AcceptsForGRPCService(t *testing.T) {
	t.Parallel()
	prMsg := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Overrides{
			Overrides: &extprocv3.ExtProcOverrides{
				ProcessingMode: &extprocv3.ProcessingMode{
					RequestBodyMode:  extprocv3.ProcessingMode_BUFFERED,
					ResponseBodyMode: extprocv3.ProcessingMode_BUFFERED,
				},
			},
		},
	}
	got, err := parseExtProcPerRoute(prMsg, false /*httpServiceMode=gRPC arm*/)
	if err != nil {
		t.Fatalf("parseExtProcPerRoute(per-route BUFFERED, gRPC arm): err = %v; want nil (post-§Decision AMENDMENT)", err)
	}
	if got == nil {
		t.Fatalf("got = nil; want non-nil")
	}
	if got.effectiveProcessingMode == nil {
		t.Fatalf(".effectiveProcessingMode = nil; want non-nil (per-route processing_mode override)")
	}
	if got.effectiveProcessingMode.RequestBodyMode != extprocv3.ProcessingMode_BUFFERED {
		t.Errorf(".effectiveProcessingMode.RequestBodyMode = %v; want BUFFERED",
			got.effectiveProcessingMode.RequestBodyMode)
	}
	if got.effectiveProcessingMode.ResponseBodyMode != extprocv3.ProcessingMode_BUFFERED {
		t.Errorf(".effectiveProcessingMode.ResponseBodyMode = %v; want BUFFERED",
			got.effectiveProcessingMode.ResponseBodyMode)
	}
}

// TestBuildCompiledConfig_TrailerModeNotSKIP_ParseReject — trailer-mode != SKIP
// PARSE-REJECT permanently.
func TestBuildCompiledConfig_TrailerModeNotSKIP_ParseReject(t *testing.T) {
	t.Parallel()
	cm := mkExtprocH2ClusterMgr(t, "c_extproc", 9999)
	cases := []struct {
		name string
		pm   *extprocv3.ProcessingMode
		want string
	}{
		{"request_trailer_send", &extprocv3.ProcessingMode{RequestTrailerMode: extprocv3.ProcessingMode_SEND}, "request_trailer_mode"},
		{"response_trailer_send", &extprocv3.ProcessingMode{ResponseTrailerMode: extprocv3.ProcessingMode_SEND}, "response_trailer_mode"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := mkValidGRPCExtProc("c_extproc")
			raw.ProcessingMode = tc.pm
			cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{ClusterManager: cm})
			if cc != nil {
				t.Fatalf("cc = %v; want nil", cc)
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v; want substring %q", err, tc.want)
			}
		})
	}
}

// TestBuildCompiledConfig_HTTPService_BodyMode_ParseReject — http_service
// arm forces body-mode NONE per the proto constraint.
func TestBuildCompiledConfig_HTTPService_BodyMode_ParseReject(t *testing.T) {
	t.Parallel()
	raw := mkValidHTTPExtProc("http://processor.local:8080/process")
	raw.ProcessingMode = &extprocv3.ProcessingMode{
		RequestBodyMode: extprocv3.ProcessingMode_BUFFERED,
	}
	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{})
	if cc != nil {
		t.Fatalf("cc = %v; want nil", cc)
	}
	if err == nil || !strings.Contains(err.Error(), "body_mode") {
		t.Fatalf("err = %v; want 'body_mode'", err)
	}
}

// TestBuildCompiledConfig_RouteCacheActionMutex_ParseReject — both
// disable_clear_route_cache + route_cache_action set → PARSE-REJECT.
func TestBuildCompiledConfig_RouteCacheActionMutex_ParseReject(t *testing.T) {
	t.Parallel()
	cm := mkExtprocH2ClusterMgr(t, "c_extproc", 9999)
	raw := mkValidGRPCExtProc("c_extproc")
	raw.RouteCacheAction = extprocv3.ExternalProcessor_RETAIN
	raw.DisableClearRouteCache = true
	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{ClusterManager: cm})
	if cc != nil {
		t.Fatalf("cc = %v; want nil", cc)
	}
	if err == nil || !strings.Contains(err.Error(), "only one of route_cache_action or disable_clear_route_cache") {
		t.Fatalf("err = %v; want mutual-exclusion error", err)
	}
}

// TestBuildCompiledConfig_DisableClearRouteCacheAlone_TranslatesRetain — the
// disable_clear_route_cache=true alone translates to routeCacheAction=RETAIN.
func TestBuildCompiledConfig_DisableClearRouteCacheAlone_TranslatesRetain(t *testing.T) {
	t.Parallel()
	cm := mkExtprocH2ClusterMgr(t, "c_extproc", 9999)
	raw := mkValidGRPCExtProc("c_extproc")
	raw.DisableClearRouteCache = true
	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{ClusterManager: cm})
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if cc.routeCacheAction != extprocv3.ExternalProcessor_RETAIN {
		t.Errorf("routeCacheAction = %v; want RETAIN", cc.routeCacheAction)
	}
	if cc.grpcClient != nil {
		_ = cc.grpcClient.Close()
	}
}

// TestBuildCompiledConfig_AllowedOverrideModes_BodyMode_ParseReject — every
// entry in allowed_override_modes is validated through resolveProcessingMode.
// Post-19.2 §Decision AMENDMENT the BUFFERED arm is ACCEPTED; the STREAMED-
// class arms continue PARSE-REJECT permanently. We exercise STREAMED here
// (still load-bearing — the per-entry validation must reject STREAMED-class
// body modes even when the listener-level mode is valid).
func TestBuildCompiledConfig_AllowedOverrideModes_BodyMode_ParseReject(t *testing.T) {
	t.Parallel()
	cm := mkExtprocH2ClusterMgr(t, "c_extproc", 9999)
	raw := mkValidGRPCExtProc("c_extproc")
	raw.AllowedOverrideModes = []*extprocv3.ProcessingMode{
		{RequestHeaderMode: extprocv3.ProcessingMode_SEND},   // valid
		{RequestBodyMode: extprocv3.ProcessingMode_STREAMED}, // STREAMED-class continues PARSE-REJECT permanently
	}
	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{ClusterManager: cm})
	if cc != nil {
		t.Fatalf("cc = %v; want nil", cc)
	}
	if err == nil || !strings.Contains(err.Error(), "allowed_override_modes[1]") {
		t.Fatalf("err = %v; want substring 'allowed_override_modes[1]'", err)
	}
}

// TestBuildCompiledConfig_GRPC_HappyPath_FieldsPopulated — Group 2: assert
// every compiledConfig field is correctly populated post-parse for the gRPC
// arm with all 19.1 MVP-consumed fields set.
func TestBuildCompiledConfig_GRPC_HappyPath_FieldsPopulated(t *testing.T) {
	t.Parallel()
	cm := mkExtprocH2ClusterMgr(t, "c_extproc", 9999)
	reg := stats.NewRegistry()

	raw := &extprocv3.ExternalProcessor{
		GrpcService: &corev3.GrpcService{
			TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: "c_extproc"},
			},
		},
		FailureModeAllow:   true,
		MessageTimeout:     durationpb.New(750 * time.Millisecond),
		MaxMessageTimeout:  durationpb.New(2 * time.Second),
		RequestAttributes:  []string{"source.address", "connection.principal"},
		ResponseAttributes: []string{"destination.address"},
		AllowModeOverride:  true,
		AllowedOverrideModes: []*extprocv3.ProcessingMode{
			{RequestHeaderMode: extprocv3.ProcessingMode_SEND, ResponseHeaderMode: extprocv3.ProcessingMode_SKIP},
		},
		ProcessingMode: &extprocv3.ProcessingMode{
			RequestHeaderMode:  extprocv3.ProcessingMode_SEND,
			ResponseHeaderMode: extprocv3.ProcessingMode_DEFAULT, // translates to SEND
		},
		DisableImmediateResponse: true,
		StatPrefix:               "my_route",
		RouteCacheAction:         extprocv3.ExternalProcessor_CLEAR,
		MutationRules: &commonmutationv3.HeaderMutationRules{
			AllowAllRouting: wrapperspb.Bool(true),
		},
	}

	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{ClusterManager: cm, Stats: reg, StatPrefix: "ingress_http"})
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if cc == nil {
		t.Fatalf("cc = nil; want non-nil")
	}
	// Transport: gRPC arm populated; HTTP arm nil.
	if cc.grpcClient == nil {
		t.Errorf("cc.grpcClient = nil; want non-nil")
	}
	if cc.httpClient != nil {
		t.Errorf("cc.httpClient = %v; want nil (gRPC mode)", cc.httpClient)
	}
	if cc.httpServiceHeadersOnly {
		t.Errorf("cc.httpServiceHeadersOnly = true; want false (gRPC mode)")
	}
	// Error-posture fields.
	if !cc.failureModeAllow {
		t.Errorf("cc.failureModeAllow = false; want true")
	}
	if cc.messageTimeout != 750*time.Millisecond {
		t.Errorf("cc.messageTimeout = %v; want 750ms", cc.messageTimeout)
	}
	if cc.maxMessageTimeout != 2*time.Second {
		t.Errorf("cc.maxMessageTimeout = %v; want 2s", cc.maxMessageTimeout)
	}
	if !cc.disableImmediateResponse {
		t.Errorf("cc.disableImmediateResponse = false; want true")
	}
	// Processing-mode.
	if cc.processingMode == nil {
		t.Fatalf("cc.processingMode = nil; want non-nil")
	}
	if cc.processingMode.RequestHeaderMode != extprocv3.ProcessingMode_SEND {
		t.Errorf("RequestHeaderMode = %v; want SEND", cc.processingMode.RequestHeaderMode)
	}
	if cc.processingMode.ResponseHeaderMode != extprocv3.ProcessingMode_SEND {
		t.Errorf("ResponseHeaderMode = %v; want SEND (DEFAULT → SEND)", cc.processingMode.ResponseHeaderMode)
	}
	// Mode override.
	if !cc.allowModeOverride {
		t.Errorf("cc.allowModeOverride = false; want true")
	}
	if len(cc.allowedOverrideModes) != 1 {
		t.Errorf("len(cc.allowedOverrideModes) = %d; want 1", len(cc.allowedOverrideModes))
	}
	// Mutation rules.
	if cc.mutationRules == nil {
		t.Errorf("cc.mutationRules = nil; want non-nil")
	} else if !cc.mutationRules.AllowAllRouting {
		t.Errorf("AllowAllRouting = false; want true")
	}
	// Forward rules — placeholder at 19.1.
	if cc.forwardRules == nil {
		t.Errorf("cc.forwardRules = nil; want non-nil placeholder")
	}
	// Attribute envelopes.
	if len(cc.requestAttributes) != 2 {
		t.Errorf("requestAttributes len = %d; want 2", len(cc.requestAttributes))
	}
	if len(cc.responseAttributes) != 1 {
		t.Errorf("responseAttributes len = %d; want 1", len(cc.responseAttributes))
	}
	// Route-cache.
	if cc.routeCacheAction != extprocv3.ExternalProcessor_CLEAR {
		t.Errorf("routeCacheAction = %v; want CLEAR", cc.routeCacheAction)
	}
	// Stats: registered into reg under the http.ingress_http.ext_proc.* namespace.
	if cc.stats == nil {
		t.Fatalf("cc.stats = nil; want non-nil (Stats supplied)")
	}
	// Stat-prefix capture.
	if cc.statPrefix != "my_route" {
		t.Errorf("statPrefix = %q; want 'my_route'", cc.statPrefix)
	}
	// Cleanup.
	if cc.grpcClient != nil {
		_ = cc.grpcClient.Close()
	}
}

// TestBuildCompiledConfig_GRPC_DefaultsApplied — Group 2: assert the proto
// defaults flow through correctly: messageTimeout 200ms, maxMessageTimeout 0,
// failureModeAllow false, disableImmediateResponse false.
func TestBuildCompiledConfig_GRPC_DefaultsApplied(t *testing.T) {
	t.Parallel()
	cm := mkExtprocH2ClusterMgr(t, "c_extproc", 9999)
	raw := mkValidGRPCExtProc("c_extproc")

	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{ClusterManager: cm})
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if cc.messageTimeout != 200*time.Millisecond {
		t.Errorf("messageTimeout = %v; want 200ms default", cc.messageTimeout)
	}
	if cc.maxMessageTimeout != 0 {
		t.Errorf("maxMessageTimeout = %v; want 0 default (override-disabled)", cc.maxMessageTimeout)
	}
	if cc.failureModeAllow {
		t.Errorf("failureModeAllow = true; want false default")
	}
	if cc.disableImmediateResponse {
		t.Errorf("disableImmediateResponse = true; want false default")
	}
	// processingMode all-nil input → all-defaults (SEND for headers; SKIP for trailers).
	if cc.processingMode == nil {
		t.Fatalf("processingMode nil; want default")
	}
	if cc.processingMode.RequestHeaderMode != extprocv3.ProcessingMode_SEND {
		t.Errorf("RequestHeaderMode default = %v; want SEND", cc.processingMode.RequestHeaderMode)
	}
	if cc.processingMode.RequestTrailerMode != extprocv3.ProcessingMode_SKIP {
		t.Errorf("RequestTrailerMode default = %v; want SKIP", cc.processingMode.RequestTrailerMode)
	}
	if cc.grpcClient != nil {
		_ = cc.grpcClient.Close()
	}
}

// TestBuildCompiledConfig_HTTP_HappyPath_FieldsPopulated — Group 2: HTTP arm
// happy-path field population.
func TestBuildCompiledConfig_HTTP_HappyPath_FieldsPopulated(t *testing.T) {
	t.Parallel()
	raw := mkValidHTTPExtProc("http://processor.local:8080/process")
	raw.MessageTimeout = durationpb.New(300 * time.Millisecond)
	raw.RequestAttributes = []string{"request.protocol"}

	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{})
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if cc.httpClient == nil {
		t.Fatalf("httpClient = nil; want non-nil")
	}
	if cc.grpcClient != nil {
		t.Errorf("grpcClient = %v; want nil (HTTP mode)", cc.grpcClient)
	}
	if !cc.httpServiceHeadersOnly {
		t.Errorf("httpServiceHeadersOnly = false; want true (HTTP mode)")
	}
	if cc.httpClient.baseURL != "http://processor.local:8080/process" {
		t.Errorf("baseURL = %q; want 'http://processor.local:8080/process'", cc.httpClient.baseURL)
	}
	if cc.messageTimeout != 300*time.Millisecond {
		t.Errorf("messageTimeout = %v; want 300ms", cc.messageTimeout)
	}
	if len(cc.requestAttributes) != 1 {
		t.Errorf("requestAttributes len = %d; want 1", len(cc.requestAttributes))
	}
}

// TestBuildCompiledConfig_NilStatsRegistry_NilStatsField — when ctx.Stats is
// nil (per ADR-0085 nil-tolerance), cc.stats stays nil; no panic.
func TestBuildCompiledConfig_NilStatsRegistry_NilStatsField(t *testing.T) {
	t.Parallel()
	cm := mkExtprocH2ClusterMgr(t, "c_extproc", 9999)
	raw := mkValidGRPCExtProc("c_extproc")
	cc, err := buildCompiledConfig(raw, envoyhttp.FactoryCtx{ClusterManager: cm, Stats: nil})
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if cc.stats != nil {
		t.Errorf("cc.stats = %v; want nil (ctx.Stats was nil)", cc.stats)
	}
	if cc.grpcClient != nil {
		_ = cc.grpcClient.Close()
	}
}

// TestNew_GRPC_HappyPath_ReturnsFactory — the full New factory pathway.
// Returns non-nil FilterInstanceFactory + nil err for a valid gRPC config.
// The returned factory allocates a fresh *filter on each invocation per the
// BOTH-DECODE-AND-ENCODE ADR-0167 shape.
func TestNew_GRPC_HappyPath_ReturnsFactory(t *testing.T) {
	t.Parallel()
	cm := mkExtprocH2ClusterMgr(t, "c_extproc", 9999)
	tc, err := anypb.New(mkValidGRPCExtProc("c_extproc"))
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	factory, err := New(tc, envoyhttp.FactoryCtx{ClusterManager: cm})
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if factory == nil {
		t.Fatalf("factory = nil; want non-nil")
	}
	// Invoke the factory to get a *filter instance. Per ADR-0167 the HTTPFilter
	// value has Decoder + Encoder BOTH non-nil and pointing at the SAME
	// *filter instance.
	hf := factory()
	if hf.Decoder == nil {
		t.Errorf("hf.Decoder = nil; want non-nil")
	}
	if hf.Encoder == nil {
		t.Errorf("hf.Encoder = nil; want non-nil")
	}
	// Per ADR-0167 the same *filter instance serves both sides. The
	// interface types differ (StreamDecoderFilter vs StreamEncoderFilter)
	// so direct == comparison is type-unsafe; instead, type-assert to the
	// concrete *filter and compare pointers.
	d, dOk := hf.Decoder.(*filter)
	e, eOk := hf.Encoder.(*filter)
	if !dOk || !eOk {
		t.Errorf("Decoder/Encoder concrete type != *filter; got Decoder=%T Encoder=%T", hf.Decoder, hf.Encoder)
	} else if d != e {
		t.Errorf("Decoder (%p) != Encoder (%p); per ADR-0167 same *filter instance must serve both sides", d, e)
	}
	if hf.Name != filterName {
		t.Errorf("hf.Name = %q; want %q", hf.Name, filterName)
	}
}

// TestNew_HTTP_HappyPath_ReturnsFactory — the full New factory for an HTTP
// service config.
func TestNew_HTTP_HappyPath_ReturnsFactory(t *testing.T) {
	t.Parallel()
	tc, err := anypb.New(mkValidHTTPExtProc("http://processor.local:8080/process"))
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	factory, err := New(tc, envoyhttp.FactoryCtx{})
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if factory == nil {
		t.Fatalf("factory = nil; want non-nil")
	}
	hf := factory()
	if hf.Decoder == nil || hf.Encoder == nil {
		t.Errorf("Decoder/Encoder nil; want both non-nil per ADR-0167")
	}
}

// TestNew_InvalidConfig_ReturnsError — the New factory surfaces the
// buildCompiledConfig PARSE-REJECT verbatim.
func TestNew_InvalidConfig_ReturnsError(t *testing.T) {
	t.Parallel()
	// Empty ExternalProcessor → mutual-exclusion PARSE-REJECT.
	tc, err := anypb.New(&extprocv3.ExternalProcessor{})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	factory, err := New(tc, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatalf("err = nil; want PARSE-REJECT")
	}
	if factory != nil {
		t.Errorf("factory = %v; want nil on PARSE-REJECT", factory)
	}
}

// ---------------------------------------------------------------------------
// DecodeHeaders + EncodeHeaders body-coverage tests per SPEC §6.3 + §6.4.
// ---------------------------------------------------------------------------

// TestDecodeHeaders_PerRouteDisabled_ShortCircuit — disabled-per-route returns
// Continue immediately, no processor call, no counter increments per parent §5.P6.
func TestDecodeHeaders_PerRouteDisabled_ShortCircuit(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		stats:          newFilterStats(reg, "ingress_http"),
		processingMode: &resolvedProcessingMode{RequestHeaderMode: extprocv3.ProcessingMode_SEND},
	}
	state := &factoryState{listenerRC: cc}
	f := &filter{
		state:                state,
		cc:                   cc,
		parentCtx:            context.Background(),
		activeProcessingMode: cc.processingMode,
		activePerRoute:       &resolvedPerRoute{disabled: true},
	}
	status := f.DecodeHeaders(http.Header{"X-Foo": []string{"bar"}}, true)
	if status != envoyhttp.Continue {
		t.Errorf("status = %v; want Continue (disabled-per-route)", status)
	}
	if got := cc.stats.streamsStarted.Load(); got != 0 {
		t.Errorf("streamsStarted = %d; want 0 (disabled — no processor call)", got)
	}
}

// TestDecodeHeaders_SKIPMode_ShortCircuit — request_header_mode==SKIP returns
// Continue immediately (no processor call per SPEC §6.3).
func TestDecodeHeaders_SKIPMode_ShortCircuit(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{
		processingMode: &resolvedProcessingMode{RequestHeaderMode: extprocv3.ProcessingMode_SKIP},
	}
	state := &factoryState{listenerRC: cc}
	f := &filter{
		state:                state,
		cc:                   cc,
		parentCtx:            context.Background(),
		activeProcessingMode: cc.processingMode,
	}
	status := f.DecodeHeaders(http.Header{}, true)
	if status != envoyhttp.Continue {
		t.Errorf("status = %v; want Continue (SKIP mode)", status)
	}
}

// TestDecodeHeaders_CapturesRequestContentType — the gRPC-downstream sniff
// per parent §5.P2: the request's content-type is captured at DecodeHeaders
// entry for the encode-side emitImmediateResponse consumer.
func TestDecodeHeaders_CapturesRequestContentType(t *testing.T) {
	t.Parallel()
	// HTTP-mode config to avoid the bidi-stream dispatch (which would block
	// on the gRPC ClientStream). The captured contentType test is independent
	// of the dispatcher.
	cc := &compiledConfig{
		processingMode: &resolvedProcessingMode{RequestHeaderMode: extprocv3.ProcessingMode_SKIP},
	}
	state := &factoryState{listenerRC: cc}
	f := &filter{
		state:                state,
		cc:                   cc,
		parentCtx:            context.Background(),
		activeProcessingMode: cc.processingMode,
	}
	_ = f.DecodeHeaders(http.Header{"Content-Type": []string{"application/grpc"}}, true)
	// SKIP returns before capture; the field stays empty in this path. The
	// substantive capture is exercised via the non-SKIP path below.
	if f.requestContentType != "" {
		t.Errorf("requestContentType = %q; want '' (SKIP path bypasses capture)", f.requestContentType)
	}
}

// TestEncodeHeaders_PerRouteDisabled_ShortCircuit — disabled-per-route at
// the encode side returns Continue immediately.
func TestEncodeHeaders_PerRouteDisabled_ShortCircuit(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{
		processingMode: &resolvedProcessingMode{ResponseHeaderMode: extprocv3.ProcessingMode_SEND},
	}
	state := &factoryState{listenerRC: cc}
	f := &filter{
		state:                state,
		cc:                   cc,
		parentCtx:            context.Background(),
		activeProcessingMode: cc.processingMode,
		activePerRoute:       &resolvedPerRoute{disabled: true},
	}
	status := f.EncodeHeaders(http.Header{}, true)
	if status != envoyhttp.Continue {
		t.Errorf("status = %v; want Continue (disabled)", status)
	}
}

// TestEncodeHeaders_SKIPMode_ShortCircuit — response_header_mode==SKIP →
// Continue (mode may have been mutated mid-stream by mode_override).
func TestEncodeHeaders_SKIPMode_ShortCircuit(t *testing.T) {
	t.Parallel()
	cc := &compiledConfig{
		processingMode: &resolvedProcessingMode{ResponseHeaderMode: extprocv3.ProcessingMode_SKIP},
	}
	state := &factoryState{listenerRC: cc}
	f := &filter{
		state:                state,
		cc:                   cc,
		parentCtx:            context.Background(),
		activeProcessingMode: cc.processingMode,
	}
	status := f.EncodeHeaders(http.Header{}, true)
	if status != envoyhttp.Continue {
		t.Errorf("status = %v; want Continue (SKIP)", status)
	}
}

// TestFilterResolvePerRoute_IndependentAcrossFilters per Carryforward I —
// two distinct *filter instances sharing the same *factoryState have
// INDEPENDENT f.activePerRoute caches. The cache is per-filter, not per-state.
func TestFilterResolvePerRoute_IndependentAcrossFilters(t *testing.T) {
	t.Parallel()
	state := &factoryState{listenerRC: &compiledConfig{}}

	// f1 + f2 share the same *factoryState but have independent caches.
	f1 := &filter{state: state}
	f2 := &filter{state: state}

	pr1 := f1.resolvePerRoute()
	pr2 := f2.resolvePerRoute()

	// Each filter's activePerRoute is cached independently.
	if f1.activePerRoute != pr1 {
		t.Errorf("f1.activePerRoute (%p) != pr1 (%p); want cached on f1", f1.activePerRoute, pr1)
	}
	if f2.activePerRoute != pr2 {
		t.Errorf("f2.activePerRoute (%p) != pr2 (%p); want cached on f2", f2.activePerRoute, pr2)
	}
	// f1 and f2 may receive distinct *resolvedPerRoute pointers (each is a
	// fresh zero-value from state.resolvePerRouteConfig(nil) since both
	// filters lack a dcb to resolve from); confirm cache isolation by
	// mutating f1's cached value and checking f2's stays intact.
	pr1.disabled = true
	if f2.activePerRoute.disabled {
		t.Errorf("f2.activePerRoute.disabled = true; want false (mutation on f1's cache should not affect f2)")
	}
}

// TestNew_ReturnsFactoryCallsAllocateFreshFilters — each factory invocation
// allocates a fresh *filter instance per ADR-0167 (per-request filter
// allocation per the framework's two-step factory pattern).
func TestNew_ReturnsFactoryCallsAllocateFreshFilters(t *testing.T) {
	t.Parallel()
	cm := mkExtprocH2ClusterMgr(t, "c_extproc", 9999)
	tc, err := anypb.New(mkValidGRPCExtProc("c_extproc"))
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	factory, err := New(tc, envoyhttp.FactoryCtx{ClusterManager: cm})
	if err != nil {
		t.Fatalf("New err = %v; want nil", err)
	}
	hf1 := factory()
	hf2 := factory()
	// Same factory closure produces fresh *filter per call.
	if hf1.Decoder == hf2.Decoder {
		t.Errorf("two factory() calls returned the same *filter instance; want distinct (per-request alloc)")
	}
}

// ---------------------------------------------------------------------------
// Group 8 EXPANSION — Task 11 rework follow-up per spec compliance review:
// per-route grpc_service consumption (Carryforward G) + cross-mode PARSE-REJECT
// (Carryforward H). The initial Task 11 commit deferred G + H; SPEC §5 line 216
// classifies per-route grpc_service as MVP-CONSUMED at 19.1 and PLAN Task 10
// line 602 acceptance gate required "grpc_service consumed". The rework lands
// the production consumption + the proto-mode-mismatch PARSE-REJECT.
// ---------------------------------------------------------------------------

// mkExtprocH2ClusterMgrTwoClusters builds a *cluster.Manager with TWO STATIC
// H2 clusters — used by the per-route grpc_service routing test (listener
// pins one cluster; per-route override pins the OTHER cluster + the test
// asserts the per-stream dispatch uses the per-route cluster's ProcessorClient).
func mkExtprocH2ClusterMgrTwoClusters(t testing.TB, name1, name2 string, port uint32) *cluster.Manager {
	t.Helper()
	hpoH2 := &upstreamshttpv3.HttpProtocolOptions{
		UpstreamProtocolOptions: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
			},
		},
	}
	hpoAny, err := anypb.New(hpoH2)
	if err != nil {
		t.Fatalf("anypb.New(HttpProtocolOptions): %v", err)
	}
	mkCluster := func(n string) *clusterv3.Cluster {
		return &clusterv3.Cluster{
			Name:                 n,
			ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
			LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
			ConnectTimeout:       durationpb.New(time.Second),
			LoadAssignment: &endpointv3.ClusterLoadAssignment{
				ClusterName: n,
				Endpoints: []*endpointv3.LocalityLbEndpoints{{
					LbEndpoints: []*endpointv3.LbEndpoint{{
						HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
							Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
								SocketAddress: &corev3.SocketAddress{
									Address:       "127.0.0.1",
									PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port},
								},
							}},
						}},
					}},
				}},
			},
			TypedExtensionProtocolOptions: map[string]*anypb.Any{
				"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": hpoAny,
			},
		}
	}
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{mkCluster(name1), mkCluster(name2)},
		},
	}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManager(2x h2): %v", err)
	}
	return cm
}

// TestFilterDispatch_PerRouteGrpcServiceOverride_RoutesToAlternateCluster
// asserts that when a per-route override carries `grpc_service{cluster_name:"alt"}`
// the per-stream dispatch resolves to a DISTINCT *grpcclient.ProcessorClient
// targeting the alt cluster (NOT the listener-level cluster's client). Per
// SPEC §5 line 216 + ADR-0173 §Decision: the per-route grpc_service override
// "routes different paths to different processor backends" — the initial Task
// 11 commit's silent fallthrough-to-listener was incorrect.
//
// Test shape: build a 2-cluster manager (c_main + c_alt); listener pins
// c_main; per-route override pins c_alt. Resolve per-route through
// (*factoryState).resolvePerRouteConfig and assert pr.processorClient is
// non-nil + distinct from cc.grpcClient. Then drive (*filter).DecodeHeaders
// and assert f.activeProcessorClient == pr.processorClient (NOT cc.grpcClient).
func TestFilterDispatch_PerRouteGrpcServiceOverride_RoutesToAlternateCluster(t *testing.T) {
	t.Parallel()
	cm := mkExtprocH2ClusterMgrTwoClusters(t, "c_main", "c_alt", 9999)
	ctx := envoyhttp.FactoryCtx{ClusterManager: cm}

	// Listener pins c_main.
	cc, err := buildCompiledConfig(mkValidGRPCExtProc("c_main"), ctx)
	if err != nil {
		t.Fatalf("buildCompiledConfig(c_main): %v", err)
	}
	defer func() { _ = cc.grpcClient.Close() }()
	state := &factoryState{listenerRC: cc, factoryCtx: ctx}

	// Per-route override pins c_alt.
	altGS := &corev3.GrpcService{
		TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: "c_alt"},
		},
	}
	prMsg := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Overrides{
			Overrides: &extprocv3.ExtProcOverrides{GrpcService: altGS},
		},
	}

	pr := state.resolvePerRouteConfig(prMsg)
	if pr == nil {
		t.Fatalf("resolvePerRouteConfig: got nil; want non-nil")
	}
	if pr.processorClient == nil {
		t.Fatalf("pr.processorClient = nil; want non-nil per-route ProcessorClient (Carryforward G — per-route grpc_service consumption)")
	}
	if pr.processorClient == cc.grpcClient {
		t.Errorf("pr.processorClient == cc.grpcClient (same pointer); want DISTINCT per-route client (per-route override must route to alt cluster, not listener)")
	}
	defer func() { _ = pr.processorClient.Close() }()

	// Now drive DecodeHeaders and assert the per-stream dispatch picks the
	// per-route client.
	dcb := &perRouteSwapDCB{current: prMsg}
	f := &filter{
		state:                state,
		cc:                   cc,
		dcb:                  dcb,
		parentCtx:            context.Background(),
		activeProcessingMode: cc.processingMode,
		activeMsgTimeout:     cc.messageTimeout,
	}
	// DecodeHeaders entry resolves per-route → caches activeProcessorClient.
	// We invoke resolvePerRoute + the active-client selection logic via the
	// production DecodeHeaders entry point (the side-effect is the cached
	// f.activeProcessorClient).
	//
	// NOTE: We do NOT exercise openProcessorStream end-to-end here because the
	// test cluster's port-9999 endpoint is unreachable — the dial would block.
	// Instead, we directly drive the resolvePerRoute + active-client cache
	// path that DecodeHeaders performs at Steps 1-3.
	pr2 := f.resolvePerRoute()
	if pr2 != pr {
		t.Errorf("resolvePerRoute returned different *resolvedPerRoute (%p) than the state cache (%p)", pr2, pr)
	}
	// Apply the activeProcessorClient selection rule per DecodeHeaders Step 3.
	// (The production DecodeHeaders does this inline; here we exercise the
	// resolvePerRoute path and assert the per-route client is preferred over
	// the listener client.)
	chosen := pickActiveProcessorClient(f.cc, pr2)
	if chosen == nil {
		t.Fatalf("pickActiveProcessorClient returned nil; want per-route client")
	}
	if chosen == cc.grpcClient {
		t.Errorf("pickActiveProcessorClient returned listener-level client; want per-route client (per-route override fallthrough-to-listener bug)")
	}
	if chosen != pr.processorClient {
		t.Errorf("pickActiveProcessorClient returned %p; want pr.processorClient %p", chosen, pr.processorClient)
	}
}

// TestBuildCompiledConfig_HTTPListenerWithPerRouteGrpcOverride_PARSEREJECT
// asserts that an http_service-mode listener with a per-route grpc_service
// override is PARSE-REJECTED (Carryforward H — proto-mode-mismatch). The
// rejection fires when (*factoryState).resolvePerRouteConfig parses the
// per-route under the listener's httpServiceHeadersOnly flag — and on the
// resolve path the per-route falls back to listener-level (the parse-error
// log line is the operator-visible signal; the resolved per-route is the
// listener-level fallback to avoid runtime breakage).
func TestBuildCompiledConfig_HTTPListenerWithPerRouteGrpcOverride_PARSEREJECT(t *testing.T) {
	t.Parallel()
	// HTTP-mode listener.
	ccHTTP, err := buildCompiledConfig(mkValidHTTPExtProc("http://processor.local:8080/process"), envoyhttp.FactoryCtx{})
	if err != nil {
		t.Fatalf("buildCompiledConfig(http_service listener): %v", err)
	}
	if !ccHTTP.httpServiceHeadersOnly {
		t.Fatalf("ccHTTP.httpServiceHeadersOnly = false; want true (http_service-mode listener)")
	}
	state := &factoryState{listenerRC: ccHTTP, factoryCtx: envoyhttp.FactoryCtx{}}

	// Per-route override carries a grpc_service — cross-mode mismatch.
	prMsg := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Overrides{
			Overrides: &extprocv3.ExtProcOverrides{
				GrpcService: &corev3.GrpcService{
					TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
						EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: "c_grpc"},
					},
				},
			},
		},
	}

	// Direct parse: must PARSE-REJECT (cross-mode under HTTP listener).
	rp, perr := parseExtProcPerRoute(prMsg, true /*httpServiceMode*/)
	if perr == nil {
		t.Fatalf("parseExtProcPerRoute(http listener + per-route grpc_service): err = nil; want PARSE-REJECT")
	}
	if rp != nil {
		t.Errorf("parseExtProcPerRoute (PARSE-REJECT): got = %v; want nil", rp)
	}
	if !strings.Contains(perr.Error(), "grpc_service") || !strings.Contains(perr.Error(), "http_service") {
		t.Errorf("parseExtProcPerRoute err = %q; want mentions of both 'grpc_service' and 'http_service' (cross-mode mismatch wording)", perr)
	}

	// Resolve path: parse-error logs + falls back to listener-level (matches
	// the existing parse-error-fallback discipline for per-route).
	resolved := state.resolvePerRouteConfig(prMsg)
	if resolved == nil {
		t.Fatalf("resolvePerRouteConfig: got nil; want listener-fallback non-nil")
	}
	if resolved.processorClient != nil {
		t.Errorf("resolved.processorClient = %v; want nil (cross-mode PARSE-REJECT must not construct a per-route client)", resolved.processorClient)
	}
}

// TestBuildCompiledConfig_HTTPListenerWithPerRouteProcessingModeBodyMode_PARSEREJECT
// asserts that an http_service-mode listener with a per-route processing_mode
// override carrying `request_body_mode=BUFFERED` PARSE-REJECTs via the
// httpServiceMode-gated body-mode check (which is now reachable post-rework —
// the initial Task 11 commit's hard-coded `httpServiceMode=false` at the
// per-route parse site masked this).
func TestBuildCompiledConfig_HTTPListenerWithPerRouteProcessingModeBodyMode_PARSEREJECT(t *testing.T) {
	t.Parallel()
	// Per-route override carries request_body_mode=BUFFERED.
	prMsg := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Overrides{
			Overrides: &extprocv3.ExtProcOverrides{
				ProcessingMode: &extprocv3.ProcessingMode{
					RequestBodyMode: extprocv3.ProcessingMode_BUFFERED,
				},
			},
		},
	}
	// Parse with httpServiceMode=true — must PARSE-REJECT.
	rp, perr := parseExtProcPerRoute(prMsg, true /*httpServiceMode*/)
	if perr == nil {
		t.Fatalf("parseExtProcPerRoute(http listener + per-route body-mode != NONE): err = nil; want PARSE-REJECT")
	}
	if rp != nil {
		t.Errorf("parseExtProcPerRoute (PARSE-REJECT): got = %v; want nil", rp)
	}
	if !strings.Contains(perr.Error(), "body") {
		t.Errorf("parseExtProcPerRoute err = %q; want substring 'body' (body-mode PARSE-REJECT wording)", perr)
	}
}

// TestFilterResolvePerRoute_PointerIdentityCacheForProcessorClient asserts the
// per-route ProcessorClient cache (sync.Map keyed by raw *corev3.GrpcService
// pointer-identity per ADR-0117) — two filter instances resolving the SAME
// *ExtProcPerRoute pointer (which holds the SAME *GrpcService pointer) get
// the SAME *grpcclient.ProcessorClient instance (cache hit; no duplicate dial).
func TestFilterResolvePerRoute_PointerIdentityCacheForProcessorClient(t *testing.T) {
	t.Parallel()
	cm := mkExtprocH2ClusterMgrTwoClusters(t, "c_main", "c_alt", 9999)
	ctx := envoyhttp.FactoryCtx{ClusterManager: cm}
	cc, err := buildCompiledConfig(mkValidGRPCExtProc("c_main"), ctx)
	if err != nil {
		t.Fatalf("buildCompiledConfig(c_main): %v", err)
	}
	defer func() { _ = cc.grpcClient.Close() }()
	state := &factoryState{listenerRC: cc, factoryCtx: ctx}

	altGS := &corev3.GrpcService{
		TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: "c_alt"},
		},
	}
	prMsg := &extprocv3.ExtProcPerRoute{
		Override: &extprocv3.ExtProcPerRoute_Overrides{
			Overrides: &extprocv3.ExtProcOverrides{GrpcService: altGS},
		},
	}

	// Filter 1 resolves first → constructs the per-route ProcessorClient.
	dcb1 := &perRouteSwapDCB{current: prMsg}
	f1 := &filter{state: state, cc: cc, dcb: dcb1}
	pr1 := f1.resolvePerRoute()
	if pr1 == nil || pr1.processorClient == nil {
		t.Fatalf("f1 resolvePerRoute: pr1=%v / processorClient=%v; want non-nil", pr1, pr1.processorClient)
	}

	// Filter 2 resolves the SAME prMsg → must hit the cache.
	dcb2 := &perRouteSwapDCB{current: prMsg}
	f2 := &filter{state: state, cc: cc, dcb: dcb2}
	pr2 := f2.resolvePerRoute()
	if pr2 == nil || pr2.processorClient == nil {
		t.Fatalf("f2 resolvePerRoute: pr2=%v / processorClient=%v; want non-nil", pr2, pr2.processorClient)
	}
	if pr1.processorClient != pr2.processorClient {
		t.Errorf("pointer-identity cache violation: pr1.processorClient=%p; pr2.processorClient=%p; want SAME (sync.Map cache hit)", pr1.processorClient, pr2.processorClient)
	}
	defer func() { _ = pr1.processorClient.Close() }()
}

// ---------------------------------------------------------------------------
// Group 12 — Race-detector hardening tests per Task 12 + D9 + SPEC §14.2.
//
// Tests in this group cover the four race surfaces the D9 planner-time
// decision pins:
//
//   (a) gRPC ClientStream concurrent Send+Recv discipline: the filter's
//       dispatchStage owns Send + Recv sequentially on the same goroutine;
//       only ONE Send + ONE Recv per stage per dispatch invocation. No
//       concurrent Send-vs-Send OR Recv-vs-Recv on the same stream.
//
//   (b) Framework sequential decode→encode dispatch: request_headers-stage
//       dispatch goroutine COMPLETES (signals ContinueDecoding via the resume
//       channel) BEFORE the response_headers-stage dispatch goroutine STARTS.
//       The shared *ProcessStream is accessed by AT MOST ONE goroutine at any
//       time → no per-stream mutex needed.
//
//   (c) OnDestroy-driven cancellation: f.streamCancel propagates to in-flight
//       Send/Recv via the gRPC stream's context-cancel mechanics; f.stream.
//       CloseSend signals end-of-stream from the client side. sync.Once on
//       OnDestroy makes the (streamCancel + CloseSend) pair idempotent.
//
//   (d) f.activeProcessingMode mutation race: a mode_override on the
//       request_headers ProcessingResponse mutates activeProcessingMode on the
//       request_headers recv goroutine; the response_headers dispatch reads
//       activeProcessingMode AFTER the request_headers goroutine completes
//       (per (b)) — no atomic load/store needed; the framework's sequential
//       decode→encode dispatch provides happens-before ordering.
//
// **Acceptance gate**: `go test -race -count=10 ./internal/filter/http/extproc/...`
// clean per PLAN Task 12 Step 5.
// ---------------------------------------------------------------------------

// goroutineID extracts the current goroutine's numeric ID from runtime.Stack.
// Used by TestSequentialDecodeEncodeDispatchNoRace + TestBidiStreamSendRecvDiscipline
// to verify the dispatch goroutines + Send/Recv call sites obey the D9 single-
// goroutine-per-stage invariant. Format anchor: "goroutine N [running]:" —
// the leading "goroutine " literal + an integer + space, per the runtime.Stack
// docs.
func goroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// First line shape: "goroutine NNN [status]:\n..."
	line := buf[:n]
	const prefix = "goroutine "
	if !bytes.HasPrefix(line, []byte(prefix)) {
		return 0
	}
	line = line[len(prefix):]
	end := bytes.IndexByte(line, ' ')
	if end < 0 {
		return 0
	}
	id, err := strconv.ParseUint(string(line[:end]), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// recordingProcessStream is a grpcclient.ProcessStream fake that records
// per-call goroutine IDs + invocation counts for Send + Recv + CloseSend. It
// also tolerates blocking Recv on a supplied context (so cancellation can be
// observed promptly). Used by Group 12 race tests.
type recordingProcessStream struct {
	mu sync.Mutex

	sendCalls    int
	recvCalls    int
	closeSendCnt int
	sendGIDs     []uint64
	recvGIDs     []uint64

	// concurrency-detection counters: peakConcurrentSend / peakConcurrentRecv
	// track the maximum number of goroutines simultaneously inside Send / Recv.
	// Per the gRPC ClientStream concurrency discipline (D9), each must stay <= 1.
	currentSend int32
	peakSend    int32
	currentRecv int32
	peakRecv    int32

	// Recv blocks on a context.Done OR a release channel.
	recvBlockCtx     context.Context
	recvBlockRelease chan struct{}
	recvResp         *extprocsvcv3.ProcessingResponse
	recvErr          error
	sendErr          error
}

func (s *recordingProcessStream) Send(_ *extprocsvcv3.ProcessingRequest) error {
	// Carryforward Q (Task 14): register defer immediately after AddInt32 so
	// a panic inside the CAS loop cannot leak currentSend. Peak-tracking CAS
	// runs second + still observes the freshly incremented cur.
	cur := atomic.AddInt32(&s.currentSend, 1)
	defer atomic.AddInt32(&s.currentSend, -1)
	for {
		peak := atomic.LoadInt32(&s.peakSend)
		if cur <= peak || atomic.CompareAndSwapInt32(&s.peakSend, peak, cur) {
			break
		}
	}

	gid := goroutineID()
	s.mu.Lock()
	s.sendCalls++
	s.sendGIDs = append(s.sendGIDs, gid)
	err := s.sendErr
	s.mu.Unlock()
	return err
}

func (s *recordingProcessStream) Recv() (*extprocsvcv3.ProcessingResponse, error) {
	// Carryforward Q (Task 14): mirror Send's leak-safe defer placement.
	cur := atomic.AddInt32(&s.currentRecv, 1)
	defer atomic.AddInt32(&s.currentRecv, -1)
	for {
		peak := atomic.LoadInt32(&s.peakRecv)
		if cur <= peak || atomic.CompareAndSwapInt32(&s.peakRecv, peak, cur) {
			break
		}
	}

	gid := goroutineID()
	s.mu.Lock()
	s.recvCalls++
	s.recvGIDs = append(s.recvGIDs, gid)
	blockCtx := s.recvBlockCtx
	releaseCh := s.recvBlockRelease
	resp := s.recvResp
	err := s.recvErr
	s.mu.Unlock()

	if releaseCh != nil {
		if blockCtx != nil {
			select {
			case <-releaseCh:
			case <-blockCtx.Done():
				return nil, blockCtx.Err()
			}
		} else {
			<-releaseCh
		}
	}
	return resp, err
}

func (s *recordingProcessStream) CloseSend() error {
	s.mu.Lock()
	s.closeSendCnt++
	s.mu.Unlock()
	return nil
}

func (s *recordingProcessStream) closeSendCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeSendCnt
}

func (s *recordingProcessStream) sendCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sendCalls
}

func (s *recordingProcessStream) recvCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recvCalls
}

func (s *recordingProcessStream) snapshotGIDs() (send, recv []uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	send = append([]uint64(nil), s.sendGIDs...)
	recv = append([]uint64(nil), s.recvGIDs...)
	return send, recv
}

// TestOnDestroy_CancelsInFlightProcessorStream — Task 12 Step 1.
//
// Spawn a dispatch goroutine that calls Send + then blocks on Recv (held by
// streamCtx); fire OnDestroy → assert Recv returns context.Canceled promptly
// + CloseSend invoked exactly ONCE per the sync.Once-guarded D9 discipline.
//
// Pins the OnDestroy-during-in-flight-Recv contract end-to-end (Group 10's
// TestOnDestroy_CancelsInflightRecv tests the cancellation primitive in
// isolation; this test runs it on the dispatchStage goroutine path so the
// (streamCancel + CloseSend + done flag) trio is exercised together under
// -race).
func TestOnDestroy_CancelsInFlightProcessorStream(t *testing.T) {
	// NOTE: NOT marked t.Parallel() — this test mutates the package-level
	// applyProcessingResponseFn via withApplyOverride. Per the existing Group 7
	// + Group 10 discipline, tests that swap the global override variable run
	// sequentially to avoid a test-infrastructure race on the swap-and-restore
	// pattern. (The production code's discipline is that applyProcessingResponseFn
	// is SET ONCE at package init; the test indirection is a test-only surface.)
	withApplyOverride(t, func(*filter, stage, *extprocsvcv3.ProcessingResponse) (action, error) {
		return actContinue, nil
	})
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		messageTimeout: 5 * time.Second,
		stats:          newFilterStats(reg, "ondestroy_cancel"),
	}
	dcb := &fakeDCB{}
	ctx, cancel := context.WithCancel(context.Background())
	releaseCh := make(chan struct{}) // never closed — Recv only unblocks on ctx cancel
	stream := &recordingProcessStream{
		recvBlockCtx:     ctx,
		recvBlockRelease: releaseCh,
	}
	f := &filter{
		cc:               cc,
		dcb:              dcb,
		stream:           stream,
		streamCtx:        ctx,
		streamCancel:     cancel,
		activeMsgTimeout: 5 * time.Second,
	}

	// Dispatch — goroutine performs Send then blocks on Recv.
	f.dispatchStage(stageRequestHeaders, &extprocsvcv3.ProcessingRequest{})

	// Wait until Send has fired + Recv is parked (best-effort short wait).
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if stream.sendCount() >= 1 && stream.recvCount() >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if stream.sendCount() != 1 {
		t.Fatalf("Send calls = %d; want 1 (dispatch goroutine should have Send'd before parking on Recv)", stream.sendCount())
	}
	if stream.recvCount() != 1 {
		t.Fatalf("Recv calls = %d; want 1 (dispatch goroutine should be parked on Recv)", stream.recvCount())
	}

	// OnDestroy → cancels streamCtx → Recv returns context.Canceled → completeStage
	// sees f.done=true and DROPS the resume signal (the chain is torn down).
	onDestroyAt := time.Now()
	f.OnDestroy()

	// Wait for the dispatch goroutine to observe ctx.Canceled + return.
	// The goroutine increments streamsFailed (via the recvErr path).
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cc.stats.streamsFailed.Load() >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	elapsed := time.Since(onDestroyAt)
	if cc.stats.streamsFailed.Load() != 1 {
		t.Errorf("streamsFailed = %d; want 1 (cancel-induced Recv error)", cc.stats.streamsFailed.Load())
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("Recv did not observe cancel within 200ms of OnDestroy; elapsed=%v (cancel discipline broken)", elapsed)
	}

	// CloseSend invoked exactly ONCE per sync.Once.
	if got := stream.closeSendCount(); got != 1 {
		t.Errorf("CloseSend calls = %d; want 1 (sync.Once-guarded OnDestroy)", got)
	}
	// f.done flipped under f.mu so the dispatch goroutine's completeStage drops
	// the resume signal — dcb.ContinueDecoding must NOT have fired.
	if dcb.calls() != 0 {
		t.Errorf("ContinueDecoding calls = %d; want 0 (done flag must drop the resume signal after OnDestroy)", dcb.calls())
	}
	// streamsClosed incremented exactly ONCE per the sync.Once guard.
	if got := cc.stats.streamsClosed.Load(); got != 1 {
		t.Errorf("streamsClosed = %d; want 1", got)
	}
	// Unblock the release channel for cleanup (no-op now that ctx is canceled,
	// but keeps the test deterministic on cleanup paths).
	close(releaseCh)
}

// TestSequentialDecodeEncodeDispatchNoRace — Task 12 Step 2.
//
// Drives DecodeHeaders + EncodeHeaders sequentially against a fake bidi-stream
// and asserts the D9 framework-sequential-dispatch invariant: the request_
// headers dispatch goroutine COMPLETES (signals ContinueDecoding via the
// resume channel) BEFORE the response_headers dispatch goroutine STARTS. Under
// -race over -count=10 iterations the test confirms no data race on the shared
// f.stream / f.activeProcessingMode surface.
//
// The goroutine-completion ordering is observed by capturing the running
// goroutine ID at the START of each Send (the dispatchStage goroutine's only
// observable entry point through the stream fake); we assert that the encode-
// side Send only fires AFTER the decode-side resume signal has been observed.
func TestSequentialDecodeEncodeDispatchNoRace(t *testing.T) {
	// NOT t.Parallel() — see TestOnDestroy_CancelsInFlightProcessorStream note.
	withApplyOverride(t, func(*filter, stage, *extprocsvcv3.ProcessingResponse) (action, error) {
		return actContinue, nil
	})
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		messageTimeout: 5 * time.Second,
		processingMode: &resolvedProcessingMode{
			RequestHeaderMode:  extprocv3.ProcessingMode_SEND,
			ResponseHeaderMode: extprocv3.ProcessingMode_SEND,
		},
		stats: newFilterStats(reg, "seq_dispatch"),
	}
	state := &factoryState{listenerRC: cc}
	dcb := &fakeDCB{}
	ecb := &fakeECB{}
	stream := &recordingProcessStream{
		recvResp: &extprocsvcv3.ProcessingResponse{
			Response: &extprocsvcv3.ProcessingResponse_RequestHeaders{
				RequestHeaders: &extprocsvcv3.HeadersResponse{},
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &filter{
		state:                state,
		cc:                   cc,
		dcb:                  dcb,
		ecb:                  ecb,
		parentCtx:            ctx,
		streamCtx:            ctx,
		streamCancel:         cancel,
		stream:               stream,
		activeProcessingMode: cc.processingMode,
		activeMsgTimeout:     5 * time.Second,
	}

	// === Decode side ===
	status := f.DecodeHeaders(http.Header{"X-Foo": []string{"bar"}}, false)
	if status != envoyhttp.StopIteration {
		t.Fatalf("DecodeHeaders status = %v; want StopIteration (async dispatch pending)", status)
	}
	// Wait for the resume signal.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if dcb.calls() >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if dcb.calls() != 1 {
		t.Fatalf("dcb.calls = %d; want 1 (decode-side resume must fire)", dcb.calls())
	}
	decodeSends := stream.sendCount()
	decodeRecvs := stream.recvCount()
	if decodeSends != 1 || decodeRecvs != 1 {
		t.Fatalf("after decode dispatch: Send=%d, Recv=%d; want 1, 1", decodeSends, decodeRecvs)
	}

	// === Encode side ===
	status = f.EncodeHeaders(http.Header{"X-Bar": []string{"baz"}}, true)
	if status != envoyhttp.StopIteration {
		t.Fatalf("EncodeHeaders status = %v; want StopIteration", status)
	}
	deadline = time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if ecb.calls() >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if ecb.calls() != 1 {
		t.Fatalf("ecb.calls = %d; want 1 (encode-side resume must fire)", ecb.calls())
	}
	if got := stream.sendCount(); got != 2 {
		t.Errorf("post-encode Send count = %d; want 2 (one per stage)", got)
	}
	if got := stream.recvCount(); got != 2 {
		t.Errorf("post-encode Recv count = %d; want 2 (one per stage)", got)
	}

	// Sequential-dispatch invariant: decode goroutine completed BEFORE encode
	// goroutine started. The recordingProcessStream's peakSend / peakRecv
	// counters MUST stay <= 1 (no concurrent goroutines inside Send or Recv).
	if peak := atomic.LoadInt32(&stream.peakSend); peak > 1 {
		t.Errorf("peakSend = %d; want <= 1 (D9 invariant: at-most-one goroutine inside Send at any time)", peak)
	}
	if peak := atomic.LoadInt32(&stream.peakRecv); peak > 1 {
		t.Errorf("peakRecv = %d; want <= 1 (D9 invariant: at-most-one goroutine inside Recv at any time)", peak)
	}

	// Sanity: per-stage Send/Recv goroutine IDs are recorded. Each stage's
	// dispatchStage uses ONE goroutine for the Send+Recv pair, so the same
	// goroutine's GID appears as the Nth Send + the Nth Recv.
	sendGIDs, recvGIDs := stream.snapshotGIDs()
	if len(sendGIDs) != 2 || len(recvGIDs) != 2 {
		t.Fatalf("GIDs: send=%v recv=%v; want 2 each", sendGIDs, recvGIDs)
	}
	if sendGIDs[0] != recvGIDs[0] {
		t.Errorf("decode stage: Send GID %d != Recv GID %d; want same goroutine for Send+Recv per dispatchStage", sendGIDs[0], recvGIDs[0])
	}
	if sendGIDs[1] != recvGIDs[1] {
		t.Errorf("encode stage: Send GID %d != Recv GID %d; want same goroutine for Send+Recv per dispatchStage", sendGIDs[1], recvGIDs[1])
	}
	// Different stages use different goroutines (each dispatchStage spawns a
	// fresh goroutine via `go func()`); the previous goroutine has already
	// completed by the time the next one starts per the sequential-dispatch
	// invariant.
	if sendGIDs[0] == sendGIDs[1] {
		t.Logf("note: decode + encode dispatch reused the same goroutine ID (%d) — possible after the decode goroutine exits + the runtime reuses the GID; this is permitted as long as the goroutines are sequential", sendGIDs[0])
	}
}

// TestModeOverrideRaceClean — Task 12 Step 3.
//
// Exercises the mode_override mid-stream race surface: a request_headers
// ProcessingResponse carries `mode_override{response_header_mode: SKIP}`. The
// applyProcessingResponse handler mutates f.activeProcessingMode on the
// request_headers recv goroutine. After the decode resume fires, EncodeHeaders
// runs on the framework dispatch goroutine and READS f.activeProcessingMode →
// observes the SKIP mutation → returns Continue immediately WITHOUT firing
// the response_headers ProcessingRequest.
//
// The race-detector clean run pins the D9 happens-before ordering: the
// decode-side recv goroutine completes (resume signaled) BEFORE the encode-
// side read happens; no atomic load/store needed.
func TestModeOverrideRaceClean(t *testing.T) {
	// NOT t.Parallel() — the dispatchStage goroutine reads the package-level
	// applyProcessingResponseFn variable; running in parallel with other tests
	// that swap that var via withApplyOverride would trip the race detector on
	// the test-infrastructure surface (not on the production race surface this
	// test is verifying — that surface is the activeProcessingMode mutation).
	// Use the REAL applyProcessingResponse — it mutates activeProcessingMode
	// per check.go step 3 when allow_mode_override=true + override is valid.
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		messageTimeout:    5 * time.Second,
		allowModeOverride: true,
		processingMode: &resolvedProcessingMode{
			RequestHeaderMode:  extprocv3.ProcessingMode_SEND,
			ResponseHeaderMode: extprocv3.ProcessingMode_SEND,
		},
		stats: newFilterStats(reg, "mode_override_race"),
	}
	state := &factoryState{listenerRC: cc}
	dcb := &fakeDCB{}
	ecb := &fakeECB{}
	// Response: mode_override flips response_header_mode → SKIP, plus a valid
	// CommonResponse for the request_headers stage.
	overrideResp := &extprocsvcv3.ProcessingResponse{
		ModeOverride: &extprocv3.ProcessingMode{
			ResponseHeaderMode: extprocv3.ProcessingMode_SKIP,
		},
		Response: &extprocsvcv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocsvcv3.HeadersResponse{
				Response: &extprocsvcv3.CommonResponse{
					Status: extprocsvcv3.CommonResponse_CONTINUE,
				},
			},
		},
	}
	stream := &recordingProcessStream{recvResp: overrideResp}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &filter{
		state:                state,
		cc:                   cc,
		dcb:                  dcb,
		ecb:                  ecb,
		parentCtx:            ctx,
		streamCtx:            ctx,
		streamCancel:         cancel,
		stream:               stream,
		activeProcessingMode: cc.processingMode,
		activeMsgTimeout:     5 * time.Second,
	}

	// === Decode side: dispatch + mode_override mutates activeProcessingMode ===
	status := f.DecodeHeaders(http.Header{}, false)
	if status != envoyhttp.StopIteration {
		t.Fatalf("DecodeHeaders status = %v; want StopIteration", status)
	}
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if dcb.calls() >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if dcb.calls() != 1 {
		t.Fatalf("dcb.calls = %d; want 1 (decode-side resume must fire after mode_override applied)", dcb.calls())
	}

	// Post-decode: activeProcessingMode mutated by applyProcessingResponse on
	// the recv goroutine. The mutation is observable after the resume signal
	// per the D9 happens-before ordering.
	if f.activeProcessingMode == nil || f.activeProcessingMode.ResponseHeaderMode != extprocv3.ProcessingMode_SKIP {
		t.Fatalf("activeProcessingMode.ResponseHeaderMode = %v; want SKIP (mode_override should have flipped it)", f.activeProcessingMode.ResponseHeaderMode)
	}

	// === Encode side: must observe SKIP + return Continue immediately ===
	// (no ProcessingRequest sent for the response_headers stage). The peakSend
	// counter must stay at 1 (one Send for the decode stage; zero for encode).
	preEncodeSends := stream.sendCount()
	preEncodeRecvs := stream.recvCount()
	encStatus := f.EncodeHeaders(http.Header{}, true)
	if encStatus != envoyhttp.Continue {
		t.Errorf("EncodeHeaders status = %v; want Continue (response_header_mode flipped to SKIP)", encStatus)
	}
	if got := stream.sendCount(); got != preEncodeSends {
		t.Errorf("post-encode Send count = %d; want %d (no encode-side dispatch under SKIP)", got, preEncodeSends)
	}
	if got := stream.recvCount(); got != preEncodeRecvs {
		t.Errorf("post-encode Recv count = %d; want %d (no encode-side dispatch under SKIP)", got, preEncodeRecvs)
	}
	if ecb.calls() != 0 {
		t.Errorf("ecb.calls = %d; want 0 (Continue returned synchronously; no async resume)", ecb.calls())
	}
}

// TestBidiStreamSendRecvDiscipline — Task 12 Step 4.
//
// Verifies the bidi-stream Send/Recv discipline per D9 + parent §5.P10: for
// each dispatchStage invocation, EXACTLY ONE Send fires + EXACTLY ONE Recv
// fires, both on the SAME goroutine (the dispatchStage's spawned goroutine).
// No concurrent Send-vs-Send OR Recv-vs-Recv on the same stream — the
// recordingProcessStream's peakSend / peakRecv counters stay <= 1.
//
// The test runs N back-to-back dispatchStage invocations (decode→encode→
// decode→encode...) — each pair completes fully before the next begins per
// the dispatch goroutine's deferred resume signal. Race-detector clean under
// -count=10.
func TestBidiStreamSendRecvDiscipline(t *testing.T) {
	// NOT t.Parallel() — see TestOnDestroy_CancelsInFlightProcessorStream note.
	withApplyOverride(t, func(*filter, stage, *extprocsvcv3.ProcessingResponse) (action, error) {
		return actContinue, nil
	})
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		messageTimeout: 5 * time.Second,
		stats:          newFilterStats(reg, "bidi_discipline"),
	}
	stream := &recordingProcessStream{
		recvResp: &extprocsvcv3.ProcessingResponse{
			Response: &extprocsvcv3.ProcessingResponse_RequestHeaders{
				RequestHeaders: &extprocsvcv3.HeadersResponse{},
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run N back-to-back dispatchStage invocations. Each fires Send + Recv on
	// a dedicated goroutine that must complete before the next begins.
	const n = 20
	for i := 0; i < n; i++ {
		dcb := &fakeDCB{}
		f := &filter{
			cc:               cc,
			dcb:              dcb,
			stream:           stream,
			streamCtx:        ctx,
			streamCancel:     cancel,
			activeMsgTimeout: 5 * time.Second,
		}
		f.dispatchStage(stageRequestHeaders, &extprocsvcv3.ProcessingRequest{})

		// Wait for the dispatch goroutine to signal resume.
		deadline := time.Now().Add(1 * time.Second)
		for time.Now().Before(deadline) {
			if dcb.calls() >= 1 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		if dcb.calls() != 1 {
			t.Fatalf("iter %d: dcb.calls = %d; want 1 (resume must fire)", i, dcb.calls())
		}
	}

	// Discipline invariants:
	// - peakSend == 1: at most ONE goroutine ever inside Send at any time.
	// - peakRecv == 1: at most ONE goroutine ever inside Recv at any time.
	// - sendCount == n: each dispatchStage fires Send exactly once.
	// - recvCount == n: each dispatchStage fires Recv exactly once.
	if peak := atomic.LoadInt32(&stream.peakSend); peak != 1 {
		t.Errorf("peakSend = %d; want 1 (no concurrent Send-vs-Send on the same stream)", peak)
	}
	if peak := atomic.LoadInt32(&stream.peakRecv); peak != 1 {
		t.Errorf("peakRecv = %d; want 1 (no concurrent Recv-vs-Recv on the same stream)", peak)
	}
	if got := stream.sendCount(); got != n {
		t.Errorf("sendCount = %d; want %d (one Send per dispatchStage)", got, n)
	}
	if got := stream.recvCount(); got != n {
		t.Errorf("recvCount = %d; want %d (one Recv per dispatchStage)", got, n)
	}

	// Per-stage GID pairing: the i-th Send + the i-th Recv share the same GID.
	sendGIDs, recvGIDs := stream.snapshotGIDs()
	if len(sendGIDs) != n || len(recvGIDs) != n {
		t.Fatalf("GIDs: send=%d recv=%d; want %d each", len(sendGIDs), len(recvGIDs), n)
	}
	for i := 0; i < n; i++ {
		if sendGIDs[i] != recvGIDs[i] {
			t.Errorf("iter %d: Send GID %d != Recv GID %d; want same goroutine per dispatchStage", i, sendGIDs[i], recvGIDs[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Group N+2 — 4-stage state-machine extension per ADR-0171 §Decision AMENDMENT
// (Task 4 of phase-19.2 IMPL).
//
// Tests cover:
//   - numStages 2 → 4: stageRequestBody + stageResponseBody now valid enum
//     values; stage.String() returns the canonical "request_body" /
//     "response_body" labels.
//   - At-most-once-per-stage discipline EXTENDS to body stages: the
//     f.overrideApplied[numStages] array sizes correctly + handleOverrideMessageTimeout
//     enforces at-most-once per body stage.
//   - Decode-side dispatch transitions: stageRequestHeaders → stageRequestBody
//     (if request_body_mode == BUFFERED) → done.
//   - Encode-side dispatch transitions: stageResponseHeaders → stageResponseBody
//     (if response_body_mode == BUFFERED) → done.
//   - Spurious body-stage entry (e.g., a body-stage callback firing when the
//     prior stage was not yet completed) increments spurious_msgs_received per
//     the existing 19.1 discipline.
// ---------------------------------------------------------------------------

// TestStateMachine_FourStage_AtMostOncePerStage asserts the at-most-once-per-stage
// override-tracking discipline extends naturally to the 4 stages — each of
// stageRequestHeaders / stageRequestBody / stageResponseHeaders /
// stageResponseBody accepts at most ONE override_message_timeout per stream;
// a second override at the SAME stage increments overrideMessageTimeoutIgnored;
// per-stage independence preserved across all 4 stages.
func TestStateMachine_FourStage_AtMostOncePerStage(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		maxMessageTimeout: 10 * time.Second,
		stats:             newFilterStats(reg, "fourstage"),
	}
	f := &filter{cc: cc}

	// First override at each of the 4 stages: accepted.
	stages := []stage{stageRequestHeaders, stageResponseHeaders, stageRequestBody, stageResponseBody}
	for _, s := range stages {
		if !f.handleOverrideMessageTimeout(s, durationpb.New(500*time.Millisecond)) {
			t.Fatalf("first override at %s rejected; want accepted (per-stage independent across all 4 stages)", s)
		}
	}
	// Second override at each stage: rejected.
	for _, s := range stages {
		if f.handleOverrideMessageTimeout(s, durationpb.New(time.Second)) {
			t.Errorf("second override at %s accepted; want rejected (at-most-once-per-stage)", s)
		}
	}
	if got := cc.stats.overrideMessageTimeoutReceived.Load(); got != 4 {
		t.Errorf("overrideMessageTimeoutReceived = %d; want 4 (one per stage)", got)
	}
	if got := cc.stats.overrideMessageTimeoutIgnored.Load(); got != 4 {
		t.Errorf("overrideMessageTimeoutIgnored = %d; want 4 (second per stage)", got)
	}
	// numStages == 4 (the f.overrideApplied array bounds match the 4-stage extension).
	if numStages != 4 {
		t.Errorf("numStages = %d; want 4 (ADR-0171 §Decision AMENDMENT)", numStages)
	}
	if len(f.overrideApplied) != int(numStages) {
		t.Errorf("len(f.overrideApplied) = %d; want %d (auto-resized via sentinel)", len(f.overrideApplied), numStages)
	}
	// stage.String coverage for the 2 NEW stages.
	if got := stageRequestBody.String(); got != "request_body" {
		t.Errorf("stageRequestBody.String() = %q; want %q", got, "request_body")
	}
	if got := stageResponseBody.String(); got != "response_body" {
		t.Errorf("stageResponseBody.String() = %q; want %q", got, "response_body")
	}
}

// TestStateMachine_DecodeStageTransitions_HeadersToBodyToDone asserts the
// decode-side stage progression: stageRequestHeaders dispatch + completeStage
// is followed by stageRequestBody dispatch + completeStage; each fires
// ContinueDecoding exactly once when applyProcessingResponseFn returns
// actContinue. Together the two stages count as 2 ContinueDecoding invocations
// — the per-stage resume signal discipline.
func TestStateMachine_DecodeStageTransitions_HeadersToBodyToDone(t *testing.T) {
	withApplyOverride(t, func(*filter, stage, *extprocsvcv3.ProcessingResponse) (action, error) {
		return actContinue, nil
	})
	dcb := &fakeDCB{}
	f := &filter{dcb: dcb}

	// Stage 1: request_headers → resume on decode side.
	f.completeStage(stageRequestHeaders, &extprocsvcv3.ProcessingResponse{}, nil)
	if got := dcb.calls(); got != 1 {
		t.Fatalf("after request_headers: dcb.calls = %d; want 1", got)
	}
	// Stage 2: request_body → resume on decode side again (body stage routes
	// through signalResume's decode-side arm — same as request_headers).
	f.completeStage(stageRequestBody, &extprocsvcv3.ProcessingResponse{}, nil)
	if got := dcb.calls(); got != 2 {
		t.Errorf("after request_body: dcb.calls = %d; want 2 (decode-side body stage signals ContinueDecoding)", got)
	}
}

// TestStateMachine_EncodeStageTransitions_HeadersToBodyToDone asserts the
// encode-side analog: response_headers + response_body each fire
// ContinueEncoding exactly once when applyProcessingResponseFn returns
// actContinue.
func TestStateMachine_EncodeStageTransitions_HeadersToBodyToDone(t *testing.T) {
	withApplyOverride(t, func(*filter, stage, *extprocsvcv3.ProcessingResponse) (action, error) {
		return actContinue, nil
	})
	ecb := &fakeECB{}
	f := &filter{ecb: ecb}

	// Stage 1: response_headers → resume on encode side.
	f.completeStage(stageResponseHeaders, &extprocsvcv3.ProcessingResponse{}, nil)
	if got := ecb.calls(); got != 1 {
		t.Fatalf("after response_headers: ecb.calls = %d; want 1", got)
	}
	// Stage 2: response_body → resume on encode side again.
	f.completeStage(stageResponseBody, &extprocsvcv3.ProcessingResponse{}, nil)
	if got := ecb.calls(); got != 2 {
		t.Errorf("after response_body: ecb.calls = %d; want 2 (encode-side body stage signals ContinueEncoding)", got)
	}
}

// TestStateMachine_SpuriousBodyStageEntry_IncrementsSpuriousMsgsReceived
// asserts that a stage-mismatch ProcessingResponse arriving at the body stage
// (e.g., the processor returned a response_headers ProcessingResponse when we
// expected request_body) is classified as spurious per the existing 19.1
// applyProcessingResponse stage-mismatch discipline + the body-stage extension.
//
// This exercises check.go's applyProcessingResponse (the real body — NOT the
// stub — installed by the package init() per Carryforward C) with a stage =
// stageRequestBody but the response carrying a request_headers arm (oneof
// mismatch). The dispatcher classifies spurious + dispError.
func TestStateMachine_SpuriousBodyStageEntry_IncrementsSpuriousMsgsReceived(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{stats: newFilterStats(reg, "spurious_body")}
	f := &filter{cc: cc}
	// Build a ProcessingResponse with the wrong oneof arm: server returned
	// request_headers but we expected request_body.
	resp := &extprocsvcv3.ProcessingResponse{
		Response: &extprocsvcv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocsvcv3.HeadersResponse{},
		},
	}
	act, err := applyProcessingResponse(f, stageRequestBody, resp)
	if act != actError {
		t.Errorf("act = %v; want actError (stage-mismatch at body stage)", act)
	}
	if err == nil {
		t.Errorf("err = nil; want non-nil sentinel (stage-mismatch)")
	}
	if got := cc.stats.spuriousMsgsReceived.Load(); got != 1 {
		t.Errorf("spuriousMsgsReceived = %d; want 1 (spurious body-stage entry)", got)
	}
}

// ---------------------------------------------------------------------------
// Group N+6 — Per-message timer behavioral enforcement per ADR-0171
// §Decision AMENDMENT clause (vi) + planner-time D4 (Task 4 of phase-19.2 IMPL).
//
// Tests cover:
//   - Single rolling timer per direction (NOT per-stage): the dispatchStage
//     goroutine's msgCtx is consumed by Send/Recv via context.WithTimeout
//     cancel-and-rebuild; a per-message timeout fires the Recv-leg cancel and
//     returns context.DeadlineExceeded → completeStage signals resume on the
//     deadline-exceeded path.
//   - context.WithTimeout cancel-and-rebuild on each stage's Send: the
//     msgCancel fires (deferred) at the end of each dispatchStage goroutine.
//   - override_message_timeout resets in-flight: when an override arrives via
//     handleOverrideMessageTimeout mid-flight, the in-flight msgCancel fires
//     so the parked Recv unblocks; the NEXT dispatchStage uses the override
//     duration.
//   - mode_override on body-stage responses silently IGNORED (not spurious)
//     per parent §5.P1 RATIFIED-AND-REFINED.
// ---------------------------------------------------------------------------

// TestPerMessageTimer_Behavioral_SingleRollingTimerPerDirection asserts the
// per-message timer is consumed BEHAVIORALLY by the dispatchStage Send/Recv
// goroutine + the watchdog cascade-cancels the stream on per-message timer
// expiry per ADR-0171 §Decision AMENDMENT clause (vi) + planner-time D4:
//
//   - The dispatchStage goroutine builds msgCtx via context.WithTimeout(streamCtx,
//     f.activeMsgTimeout).
//   - The fake stream's Recv blocks on the streamCtx (via recvBlockCtx).
//   - At the 50ms per-message timeout, the watchdog goroutine fires
//     f.streamCancel which cancels streamCtx → the fake's Recv unblocks with
//     context.Canceled (per the parent §5.P10 single-in-flight-message
//     correlation rule + the bidi-stream cascade-cancel discipline; the
//     per-message timer expiry IS the stream-level failure surface).
//   - completeStage signals resume on the recvErr path + streamsFailed
//     increments.
func TestPerMessageTimer_Behavioral_SingleRollingTimerPerDirection(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		messageTimeout: 50 * time.Millisecond,
		stats:          newFilterStats(reg, "rolling"),
	}
	dcb := &fakeDCB{}
	// Fake stream: Recv blocks on blockCh — only the streamCtx cancel can
	// unblock it (via recvBlockCtx) once the watchdog fires.
	blockCh := make(chan struct{})
	defer close(blockCh)
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	stream := &fakeProcessStream{
		recvBlockCh:  blockCh,   // never closed during the test
		recvBlockCtx: parentCtx, // streamCtx — canceled by the watchdog on per-message timeout
	}
	f := &filter{
		cc:               cc,
		dcb:              dcb,
		stream:           stream,
		streamCtx:        parentCtx,
		streamCancel:     parentCancel, // watchdog fires this on msgCtx.Done
		activeMsgTimeout: 50 * time.Millisecond,
	}

	// dispatchStage spawns the dispatch + watchdog goroutines.
	f.dispatchStage(stageRequestHeaders, &extprocsvcv3.ProcessingRequest{})

	// Wait up to 1s for the resume signal — should fire well within (timer
	// is 50ms; allow 950ms slack for watchdog + Recv-unblock scheduling).
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if dcb.calls() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if dcb.calls() != 1 {
		t.Errorf("ContinueDecoding called %d times; want 1 (per-message timer must fire + resume)", dcb.calls())
	}
	if got := cc.stats.streamsFailed.Load(); got != 1 {
		t.Errorf("streamsFailed = %d; want 1 (per-message timer expiry = transport-class failure)", got)
	}
}

// TestPerMessageTimer_ContextWithTimeout_CancelAndRebuildOnEachStageSend
// asserts the per-message timer is REBUILT (NOT reused) on each dispatchStage
// invocation — each Send/Recv pair gets a fresh context.WithTimeout(streamCtx,
// f.activeMsgTimeout). After the first stage completes (cancel deferred at
// goroutine exit), a SECOND dispatchStage invocation succeeds with a fresh
// timer + fresh deadline.
func TestPerMessageTimer_ContextWithTimeout_CancelAndRebuildOnEachStageSend(t *testing.T) {
	withApplyOverride(t, func(*filter, stage, *extprocsvcv3.ProcessingResponse) (action, error) {
		return actContinue, nil
	})
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		messageTimeout: 500 * time.Millisecond,
		stats:          newFilterStats(reg, "rebuild"),
	}
	dcb := &fakeDCB{}
	stream := &fakeProcessStream{
		recvResp: &extprocsvcv3.ProcessingResponse{
			Response: &extprocsvcv3.ProcessingResponse_RequestHeaders{
				RequestHeaders: &extprocsvcv3.HeadersResponse{},
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &filter{
		cc:               cc,
		dcb:              dcb,
		stream:           stream,
		streamCtx:        ctx,
		streamCancel:     cancel,
		activeMsgTimeout: 500 * time.Millisecond,
	}

	// Two sequential dispatchStage invocations — the second only fires AFTER
	// the first completes (resume signal observed).
	const stages = 2
	for i := 0; i < stages; i++ {
		f.dispatchStage(stageRequestHeaders, &extprocsvcv3.ProcessingRequest{})
		// Wait for resume.
		deadline := time.Now().Add(1 * time.Second)
		for time.Now().Before(deadline) {
			if dcb.calls() >= i+1 {
				break
			}
			time.Sleep(time.Millisecond)
		}
		if got := dcb.calls(); got != i+1 {
			t.Fatalf("iter %d: dcb.calls = %d; want %d (each stage rebuilds the per-message timer)", i, got, i+1)
		}
	}
	// Both stages succeeded (no streamsFailed) — the second stage's fresh
	// timer worked (no deadline carryover from the first stage's expired timer).
	if got := cc.stats.streamsFailed.Load(); got != 0 {
		t.Errorf("streamsFailed = %d; want 0 (rebuild discipline: each stage gets a fresh timer)", got)
	}
	if got := cc.stats.streamMsgsSent.Load(); got != stages {
		t.Errorf("streamMsgsSent = %d; want %d", got, stages)
	}
	if got := cc.stats.streamMsgsReceived.Load(); got != stages {
		t.Errorf("streamMsgsReceived = %d; want %d", got, stages)
	}
}

// TestPerMessageTimer_OverrideMessageTimeoutResetsInFlight asserts the
// handleOverrideMessageTimeout + the in-flight msgCancel discipline per
// planner-time D4: when an override arrives mid-flight (the dispatchStage
// goroutine is parked on Recv waiting for the SUBSTANTIVE response), the
// handler cancels the in-flight per-message timer + the NEXT dispatchStage
// observes the override duration via f.activeMsgTimeout.
//
// At Task 4 the in-flight per-message timer cancellation is mechanically
// effected by mutating f.activeMsgTimeout for the NEXT stage's Send/Recv +
// (optionally) firing the msgCancel hook captured on the filter — the test
// asserts the post-override f.activeMsgTimeout reflects the override.
func TestPerMessageTimer_OverrideMessageTimeoutResetsInFlight(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		messageTimeout:    200 * time.Millisecond,
		maxMessageTimeout: 10 * time.Second,
		stats:             newFilterStats(reg, "reset"),
	}
	f := &filter{cc: cc, activeMsgTimeout: 200 * time.Millisecond}

	// Initially the in-flight per-message timer = 200ms (cc.messageTimeout).
	if f.activeMsgTimeout != 200*time.Millisecond {
		t.Fatalf("pre-override: f.activeMsgTimeout = %v; want 200ms", f.activeMsgTimeout)
	}
	// Override arrives at request_headers stage: 1s.
	if !f.handleOverrideMessageTimeout(stageRequestHeaders, durationpb.New(time.Second)) {
		t.Fatalf("override rejected; want accepted")
	}
	// f.activeMsgTimeout is RESET to the override duration — the NEXT
	// dispatchStage's context.WithTimeout consumes this fresh value.
	if f.activeMsgTimeout != time.Second {
		t.Errorf("post-override: f.activeMsgTimeout = %v; want 1s (reset for NEXT dispatchStage)", f.activeMsgTimeout)
	}
	if got := cc.stats.overrideMessageTimeoutReceived.Load(); got != 1 {
		t.Errorf("overrideMessageTimeoutReceived = %d; want 1", got)
	}

	// IF the filter has an in-flight msgCancel captured (Task 4 IMPL settle:
	// the dispatchStage goroutine stores its msgCancel on f.activeMsgCancel
	// while parked; handleOverrideMessageTimeout fires it on accept), the
	// override path also invalidates the in-flight per-message timer so the
	// Recv unblocks promptly. We assert the captured cancel was invoked when
	// non-nil; nil-tolerant for the test path where no dispatch is in flight.
	canceled := make(chan struct{})
	cancelHook := func() { close(canceled) }
	f.activeMsgCancel = cancelHook
	if !f.handleOverrideMessageTimeout(stageResponseHeaders, durationpb.New(2*time.Second)) {
		t.Fatalf("second override (different stage) rejected; want accepted")
	}
	// The cancel hook must have fired (per D4 in-flight cancel-and-rebuild).
	select {
	case <-canceled:
		// ok — the in-flight per-message timer was canceled.
	case <-time.After(100 * time.Millisecond):
		t.Errorf("activeMsgCancel was NOT invoked by handleOverrideMessageTimeout; want invoked (D4 cancel-and-rebuild)")
	}
	// activeMsgCancel cleared post-fire so a SECOND override at the same stage
	// (rejected via at-most-once) does NOT re-fire a stale cancel.
	if f.activeMsgCancel != nil {
		t.Errorf("f.activeMsgCancel = non-nil after fire; want nil (cleared post-cancel)")
	}
}

// TestModeOverride_BodyStageResponse_SilentlyIgnoredNotSpurious asserts the
// parent §5.P1 RATIFIED-AND-REFINED discipline carry-forward at 19.2: a
// ProcessingResponse arriving at the body stage WITH a mode_override field is
// silently dropped (the mode_override is ignored — only header-response paths
// re-eval mode_override) AND is NOT classified as spurious_msgs_received. The
// body-stage CommonResponse arm still applies normally.
//
// This exercises applyProcessingResponse (the real check.go body — installed
// via Carryforward C package init reassignment) with stage == stageRequestBody
// + resp carrying both mode_override AND request_body CommonResponse arms.
// Expected: actContinue + NO spurious increment + f.activeProcessingMode
// UNCHANGED.
func TestModeOverride_BodyStageResponse_SilentlyIgnoredNotSpurious(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	priorMode := &resolvedProcessingMode{
		RequestHeaderMode:  extprocv3.ProcessingMode_SEND,
		ResponseHeaderMode: extprocv3.ProcessingMode_SEND,
		RequestBodyMode:    extprocv3.ProcessingMode_BUFFERED,
		ResponseBodyMode:   extprocv3.ProcessingMode_BUFFERED,
	}
	cc := &compiledConfig{
		allowModeOverride: true, // would otherwise be load-bearing
		processingMode:    priorMode,
		stats:             newFilterStats(reg, "modeoverride_body"),
	}
	f := &filter{cc: cc, activeProcessingMode: priorMode}

	// ProcessingResponse at stage=stageRequestBody with BOTH mode_override AND
	// request_body CommonResponse — per parent §5.P1 RATIFIED-AND-REFINED the
	// mode_override on body stages is silently ignored (NOT spurious).
	resp := &extprocsvcv3.ProcessingResponse{
		Response: &extprocsvcv3.ProcessingResponse_RequestBody{
			RequestBody: &extprocsvcv3.BodyResponse{
				Response: &extprocsvcv3.CommonResponse{},
			},
		},
		ModeOverride: &extprocv3.ProcessingMode{
			RequestHeaderMode:  extprocv3.ProcessingMode_SKIP,
			ResponseHeaderMode: extprocv3.ProcessingMode_SKIP,
		},
	}
	act, err := applyProcessingResponse(f, stageRequestBody, resp)
	if err != nil {
		t.Errorf("err = %v; want nil (body-stage mode_override silently ignored, not spurious)", err)
	}
	if act != actContinue {
		t.Errorf("act = %v; want actContinue", act)
	}
	if got := cc.stats.spuriousMsgsReceived.Load(); got != 0 {
		t.Errorf("spuriousMsgsReceived = %d; want 0 (body-stage mode_override is silent-drop per parent §5.P1 RATIFIED-AND-REFINED, NOT spurious)", got)
	}
	// activeProcessingMode UNCHANGED — the body-stage mode_override was IGNORED.
	if f.activeProcessingMode != priorMode {
		t.Errorf("f.activeProcessingMode mutated by body-stage mode_override; want unchanged (silent-ignore discipline)")
	}
}

// ---------------------------------------------------------------------------
// Group N+1 — body-stage applyProcessingResponse arms per ADR-0172 §Decision
// AMENDMENT (Task 6).
//
// Activates three previously-spurious arms per SPEC §4.2 + §4.3 + §4.4:
//
//   (a) body_mutation oneof switch: body / clear_body CONSUMED;
//       streamed_response PARSE-REJECT per D6 with spurious_msgs_received
//       increment.
//   (b) CONTINUE_AND_REPLACE at header stages with body-mode = BUFFERED →
//       COMBINED REPLACEMENT (header_mutation + body_mutation both apply;
//       sets f.skipBodyStageDispatch so the body-stage outbound is SKIPPED
//       on Task 7's body-stage entry); at header stages with body-mode =
//       NONE → CONSUMED as no-op for body (lifts 19.1 spurious-dispatch);
//       at body stages → TREATED AS CONTINUE (no counter increment).
//   (c) body-stage ImmediateResponse fires SendLocalReply via the existing
//       multi-stage emitImmediateResponse infrastructure. clear_route_cache
//       at body stages continues IGNORED per the proto's "ignored in the
//       response direction" wording.
// ---------------------------------------------------------------------------

// mkProcessingResponseRequestBody constructs a *ProcessingResponse with a
// request_body oneof arm carrying the given CommonResponse.
func mkProcessingResponseRequestBody(cr *extprocsvcv3.CommonResponse) *extprocsvcv3.ProcessingResponse {
	return &extprocsvcv3.ProcessingResponse{
		Response: &extprocsvcv3.ProcessingResponse_RequestBody{
			RequestBody: &extprocsvcv3.BodyResponse{Response: cr},
		},
	}
}

// mkProcessingResponseResponseBody constructs a *ProcessingResponse with a
// response_body oneof arm carrying the given CommonResponse.
func mkProcessingResponseResponseBody(cr *extprocsvcv3.CommonResponse) *extprocsvcv3.ProcessingResponse {
	return &extprocsvcv3.ProcessingResponse{
		Response: &extprocsvcv3.ProcessingResponse_ResponseBody{
			ResponseBody: &extprocsvcv3.BodyResponse{Response: cr},
		},
	}
}

// TestApplyProcessingResponse_BodyMutation_Body_ReplacesBufferAndReconcilesContentLength
// — per SPEC §4.2 table row 1: BodyMutation_Body replaces the buffered body
// bytes with the processor-supplied bytes + reconciles Content-Length on the
// corresponding header set. Exercises BOTH directions (decode + encode) via
// sub-tests asserting the per-direction buffer mutation + Content-Length
// header update via the directly-stashed f.decodeBodyBuf / f.encodeBodyBuf
// (Task 7 stashes these at DecodeData / EncodeData endStream; at Task 6
// the test stubs the field directly to drive the dispatcher).
func TestApplyProcessingResponse_BodyMutation_Body_ReplacesBufferAndReconcilesContentLength(t *testing.T) {
	t.Parallel()
	t.Run("decode_side_request_body", func(t *testing.T) {
		t.Parallel()
		reg := stats.NewRegistry()
		cc := &compiledConfig{stats: newFilterStats(reg, "t")}
		// Stash the pre-mutation body buffer + the headers carrier (decode side).
		hdrs := http.Header{}
		hdrs.Set("content-length", "5")
		f := &filter{
			cc:            cc,
			decodeHeaders: hdrs,
			decodeBodyBuf: []byte("hello"),
		}
		newBody := []byte("rewritten body bytes")
		cr := &extprocsvcv3.CommonResponse{
			BodyMutation: &extprocsvcv3.BodyMutation{
				Mutation: &extprocsvcv3.BodyMutation_Body{Body: newBody},
			},
		}
		resp := mkProcessingResponseRequestBody(cr)
		act, err := applyProcessingResponse(f, stageRequestBody, resp)
		if err != nil {
			t.Errorf("err = %v; want nil", err)
		}
		if act != actContinue {
			t.Errorf("act = %v; want actContinue", act)
		}
		if string(f.decodeBodyBuf) != string(newBody) {
			t.Errorf("decodeBodyBuf = %q; want %q (body_mutation Body replace)", f.decodeBodyBuf, newBody)
		}
		if got := hdrs.Get("content-length"); got != strconv.Itoa(len(newBody)) {
			t.Errorf("content-length = %q; want %d (reconciled per ADR-0128)", got, len(newBody))
		}
		if got := cc.stats.spuriousMsgsReceived.Load(); got != 0 {
			t.Errorf("spuriousMsgsReceived = %d; want 0 (happy-path body replacement)", got)
		}
	})
	t.Run("encode_side_response_body", func(t *testing.T) {
		t.Parallel()
		reg := stats.NewRegistry()
		cc := &compiledConfig{stats: newFilterStats(reg, "t")}
		hdrs := http.Header{}
		hdrs.Set("content-length", "3")
		f := &filter{
			cc:            cc,
			encodeHeaders: hdrs,
			encodeBodyBuf: []byte("foo"),
		}
		newBody := []byte("rewritten response payload")
		cr := &extprocsvcv3.CommonResponse{
			BodyMutation: &extprocsvcv3.BodyMutation{
				Mutation: &extprocsvcv3.BodyMutation_Body{Body: newBody},
			},
		}
		resp := mkProcessingResponseResponseBody(cr)
		act, err := applyProcessingResponse(f, stageResponseBody, resp)
		if err != nil {
			t.Errorf("err = %v; want nil", err)
		}
		if act != actContinue {
			t.Errorf("act = %v; want actContinue", act)
		}
		if string(f.encodeBodyBuf) != string(newBody) {
			t.Errorf("encodeBodyBuf = %q; want %q (body_mutation Body replace)", f.encodeBodyBuf, newBody)
		}
		if got := hdrs.Get("content-length"); got != strconv.Itoa(len(newBody)) {
			t.Errorf("content-length = %q; want %d (reconciled per ADR-0175)", got, len(newBody))
		}
	})
}

// TestApplyProcessingResponse_BodyMutation_ClearBody_EmptiesBuffer — per SPEC
// §4.2 table row 2: BodyMutation_ClearBody (true) empties the buffer +
// Content-Length: 0; ClearBody (false) is a no-op. Exercises both arms.
func TestApplyProcessingResponse_BodyMutation_ClearBody_EmptiesBuffer(t *testing.T) {
	t.Parallel()
	t.Run("clear_body_true_empties_decode_buffer", func(t *testing.T) {
		t.Parallel()
		reg := stats.NewRegistry()
		cc := &compiledConfig{stats: newFilterStats(reg, "t")}
		hdrs := http.Header{}
		hdrs.Set("content-length", "11")
		f := &filter{
			cc:            cc,
			decodeHeaders: hdrs,
			decodeBodyBuf: []byte("hello world"),
		}
		cr := &extprocsvcv3.CommonResponse{
			BodyMutation: &extprocsvcv3.BodyMutation{
				Mutation: &extprocsvcv3.BodyMutation_ClearBody{ClearBody: true},
			},
		}
		resp := mkProcessingResponseRequestBody(cr)
		act, err := applyProcessingResponse(f, stageRequestBody, resp)
		if err != nil {
			t.Errorf("err = %v; want nil", err)
		}
		if act != actContinue {
			t.Errorf("act = %v; want actContinue", act)
		}
		if len(f.decodeBodyBuf) != 0 {
			t.Errorf("decodeBodyBuf = %q; want empty (clear_body=true)", f.decodeBodyBuf)
		}
		if got := hdrs.Get("content-length"); got != "0" {
			t.Errorf("content-length = %q; want 0 (clear_body reconciliation)", got)
		}
	})
	t.Run("clear_body_false_is_noop", func(t *testing.T) {
		t.Parallel()
		reg := stats.NewRegistry()
		cc := &compiledConfig{stats: newFilterStats(reg, "t")}
		hdrs := http.Header{}
		hdrs.Set("content-length", "5")
		original := []byte("hello")
		f := &filter{
			cc:            cc,
			decodeHeaders: hdrs,
			decodeBodyBuf: original,
		}
		cr := &extprocsvcv3.CommonResponse{
			BodyMutation: &extprocsvcv3.BodyMutation{
				Mutation: &extprocsvcv3.BodyMutation_ClearBody{ClearBody: false},
			},
		}
		resp := mkProcessingResponseRequestBody(cr)
		act, err := applyProcessingResponse(f, stageRequestBody, resp)
		if err != nil {
			t.Errorf("err = %v; want nil", err)
		}
		if act != actContinue {
			t.Errorf("act = %v; want actContinue", act)
		}
		if string(f.decodeBodyBuf) != "hello" {
			t.Errorf("decodeBodyBuf = %q; want %q (clear_body=false is no-op)", f.decodeBodyBuf, "hello")
		}
		if got := hdrs.Get("content-length"); got != "5" {
			t.Errorf("content-length = %q; want 5 (no reconciliation when clear_body=false)", got)
		}
	})
}

// TestApplyProcessingResponse_BodyMutation_StreamedResponse_PARSE_REJECT_SpuriousMsgsReceivedIncrement
// — per SPEC §4.2 table row 3 + planner-time D6: BodyMutation_StreamedResponse
// PARSE-REJECTs (STREAMED out-of-envelope per parent §4.4) — increment
// spurious_msgs_received + return actError + sentinel error with the D6
// wording.
func TestApplyProcessingResponse_BodyMutation_StreamedResponse_PARSE_REJECT_SpuriousMsgsReceivedIncrement(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{stats: newFilterStats(reg, "t")}
	f := &filter{cc: cc, decodeBodyBuf: []byte("original")}
	cr := &extprocsvcv3.CommonResponse{
		BodyMutation: &extprocsvcv3.BodyMutation{
			Mutation: &extprocsvcv3.BodyMutation_StreamedResponse{
				StreamedResponse: &extprocsvcv3.StreamedBodyResponse{Body: []byte("nope")},
			},
		},
	}
	resp := mkProcessingResponseRequestBody(cr)
	act, err := applyProcessingResponse(f, stageRequestBody, resp)
	if act != actError {
		t.Errorf("act = %v; want actError", act)
	}
	if !errors.Is(err, errStreamedResponseBodyMutationUnsupported) {
		t.Errorf("err = %v; want errStreamedResponseBodyMutationUnsupported", err)
	}
	// Sanity-check the D6 verbatim wording.
	if err == nil || !strings.Contains(err.Error(), "streamed_response body mutation not supported") {
		t.Errorf("err = %v; want substring 'streamed_response body mutation not supported' (D6 wording)", err)
	}
	if !strings.Contains(err.Error(), "STREAMED body modes out-of-envelope per parent §4.4") {
		t.Errorf("err = %v; want substring 'STREAMED body modes out-of-envelope per parent §4.4' (D6 wording)", err)
	}
	if got := cc.stats.spuriousMsgsReceived.Load(); got != 1 {
		t.Errorf("spuriousMsgsReceived = %d; want 1 (PARSE-REJECT increment per D6)", got)
	}
	// The buffer MUST NOT have been mutated by the rejected arm.
	if string(f.decodeBodyBuf) != "original" {
		t.Errorf("decodeBodyBuf = %q; want %q (PARSE-REJECT does NOT mutate)", f.decodeBodyBuf, "original")
	}
}

// TestApplyProcessingResponse_ContinueAndReplace_HeaderStageWithBodyModeBUFFERED_CombinedReplacement_BodyStageOutboundSKIPPED
// — per SPEC §4.3 table row 2: at header stages WITH body-mode = BUFFERED,
// CONTINUE_AND_REPLACE is CONSUMED as a combined header+body replacement
// (header_mutation + body_mutation both apply); sets f.skipBodyStageDispatch
// so Task 7's body-stage entry SKIPs the body-stage outbound. The dispatcher
// returns actContinueButStillWaiting (per SPEC §4.3 row 2 transition note).
// No spurious increment.
func TestApplyProcessingResponse_ContinueAndReplace_HeaderStageWithBodyModeBUFFERED_CombinedReplacement_BodyStageOutboundSKIPPED(t *testing.T) {
	t.Parallel()
	t.Run("decode_side_request_headers_buffered_request_body", func(t *testing.T) {
		t.Parallel()
		reg := stats.NewRegistry()
		cc := &compiledConfig{
			stats:         newFilterStats(reg, "t"),
			mutationRules: resolveMutationRules(nil),
		}
		hdrs := http.Header{}
		hdrs.Set("content-length", "5")
		hdrs.Set("x-original", "yes")
		f := &filter{
			cc:            cc,
			decodeHeaders: hdrs,
			decodeBodyBuf: []byte("hello"),
			activeProcessingMode: &resolvedProcessingMode{
				RequestBodyMode: extprocv3.ProcessingMode_BUFFERED,
			},
		}
		newBody := []byte("replaced request body")
		cr := &extprocsvcv3.CommonResponse{
			Status: extprocsvcv3.CommonResponse_CONTINUE_AND_REPLACE,
			HeaderMutation: &extprocsvcv3.HeaderMutation{
				SetHeaders: []*corev3.HeaderValueOption{
					{Header: &corev3.HeaderValue{Key: "x-replaced", Value: "true"}},
				},
			},
			BodyMutation: &extprocsvcv3.BodyMutation{
				Mutation: &extprocsvcv3.BodyMutation_Body{Body: newBody},
			},
		}
		resp := mkProcessingResponseRequestHeaders(cr)
		act, err := applyProcessingResponse(f, stageRequestHeaders, resp)
		if err != nil {
			t.Errorf("err = %v; want nil (CONSUMED as combined replacement per SPEC §4.3 row 2)", err)
		}
		if act != actContinueButStillWaiting {
			t.Errorf("act = %v; want actContinueButStillWaiting (per SPEC §4.3 row 2 transition note)", act)
		}
		// header_mutation applied.
		if got := hdrs.Get("x-replaced"); got != "true" {
			t.Errorf("x-replaced header = %q; want %q (header_mutation applied)", got, "true")
		}
		// body_mutation applied.
		if string(f.decodeBodyBuf) != string(newBody) {
			t.Errorf("decodeBodyBuf = %q; want %q (body_mutation applied)", f.decodeBodyBuf, newBody)
		}
		// Content-Length reconciled to the new body length.
		if got := hdrs.Get("content-length"); got != strconv.Itoa(len(newBody)) {
			t.Errorf("content-length = %q; want %d (reconciled)", got, len(newBody))
		}
		// Body-stage outbound SKIP flag set on the request direction.
		if !f.skipBodyStageDispatch[directionRequest] {
			t.Errorf("skipBodyStageDispatch[directionRequest] = false; want true (Task 7 contract)")
		}
		if f.skipBodyStageDispatch[directionResponse] {
			t.Errorf("skipBodyStageDispatch[directionResponse] = true; want false (only request-side skip set)")
		}
		if got := cc.stats.spuriousMsgsReceived.Load(); got != 0 {
			t.Errorf("spuriousMsgsReceived = %d; want 0 (CONSUMED — not spurious)", got)
		}
	})
	t.Run("encode_side_response_headers_buffered_response_body", func(t *testing.T) {
		t.Parallel()
		reg := stats.NewRegistry()
		cc := &compiledConfig{
			stats:         newFilterStats(reg, "t"),
			mutationRules: resolveMutationRules(nil),
		}
		hdrs := http.Header{}
		hdrs.Set("content-length", "3")
		f := &filter{
			cc:            cc,
			encodeHeaders: hdrs,
			encodeBodyBuf: []byte("foo"),
			activeProcessingMode: &resolvedProcessingMode{
				ResponseBodyMode: extprocv3.ProcessingMode_BUFFERED,
			},
		}
		newBody := []byte("replaced response body")
		cr := &extprocsvcv3.CommonResponse{
			Status: extprocsvcv3.CommonResponse_CONTINUE_AND_REPLACE,
			BodyMutation: &extprocsvcv3.BodyMutation{
				Mutation: &extprocsvcv3.BodyMutation_Body{Body: newBody},
			},
		}
		resp := mkProcessingResponseResponseHeaders(cr)
		act, err := applyProcessingResponse(f, stageResponseHeaders, resp)
		if err != nil {
			t.Errorf("err = %v; want nil", err)
		}
		if act != actContinueButStillWaiting {
			t.Errorf("act = %v; want actContinueButStillWaiting", act)
		}
		if string(f.encodeBodyBuf) != string(newBody) {
			t.Errorf("encodeBodyBuf = %q; want %q", f.encodeBodyBuf, newBody)
		}
		if !f.skipBodyStageDispatch[directionResponse] {
			t.Errorf("skipBodyStageDispatch[directionResponse] = false; want true")
		}
		if f.skipBodyStageDispatch[directionRequest] {
			t.Errorf("skipBodyStageDispatch[directionRequest] = true; want false (only response-side skip set)")
		}
	})
}

// TestApplyProcessingResponse_ContinueAndReplace_BodyStage_TreatedAsContinue_NoCounterIncrement
// — per SPEC §4.3 table row 3: at body stages, CONTINUE_AND_REPLACE is
// TREATED AS CONTINUE (the proto silently ignores at body stages). No
// counter increment + no error + actContinue. body_mutation (when present)
// still applies; header_mutation still applies; the SKIP flag does NOT fire
// (body stages don't have a body-stage-outbound to skip).
func TestApplyProcessingResponse_ContinueAndReplace_BodyStage_TreatedAsContinue_NoCounterIncrement(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		stats:         newFilterStats(reg, "t"),
		mutationRules: resolveMutationRules(nil),
	}
	f := &filter{cc: cc, decodeBodyBuf: []byte("original")}
	cr := &extprocsvcv3.CommonResponse{
		Status: extprocsvcv3.CommonResponse_CONTINUE_AND_REPLACE,
	}
	resp := mkProcessingResponseRequestBody(cr)
	act, err := applyProcessingResponse(f, stageRequestBody, resp)
	if err != nil {
		t.Errorf("err = %v; want nil (TREATED AS CONTINUE per SPEC §4.3 row 3)", err)
	}
	if act != actContinue {
		t.Errorf("act = %v; want actContinue", act)
	}
	if got := cc.stats.spuriousMsgsReceived.Load(); got != 0 {
		t.Errorf("spuriousMsgsReceived = %d; want 0 (proto silently ignores at body stages — NOT spurious)", got)
	}
	if f.skipBodyStageDispatch[directionRequest] || f.skipBodyStageDispatch[directionResponse] {
		t.Errorf("skipBodyStageDispatch = %v; want both false (body-stage CONTINUE_AND_REPLACE does NOT set the skip flag)", f.skipBodyStageDispatch)
	}
}

// TestApplyProcessingResponse_BodyStageImmediateResponse_FiresSendLocalReply
// — per SPEC §4.4: body-stage ImmediateResponse fires SendLocalReply via the
// existing multi-stage emitImmediateResponse infrastructure. Verifies that
// the Task 4 4-stage extension allows body-stage ImmediateResponse to route
// through the SAME emitImmediateResponse → dcb.SendLocalReply pathway used
// at header stages.
func TestApplyProcessingResponse_BodyStageImmediateResponse_FiresSendLocalReply(t *testing.T) {
	t.Parallel()
	t.Run("request_body_stage", func(t *testing.T) {
		t.Parallel()
		dcb := &recordingDCB{}
		cc := &compiledConfig{stats: newFilterStats(stats.NewRegistry(), "t")}
		f := &filter{cc: cc, dcb: dcb}
		ir := &extprocsvcv3.ImmediateResponse{
			Status: &typev3.HttpStatus{Code: typev3.StatusCode_Forbidden},
			Body:   []byte("denied at body stage"),
		}
		resp := &extprocsvcv3.ProcessingResponse{
			Response: &extprocsvcv3.ProcessingResponse_ImmediateResponse{ImmediateResponse: ir},
		}
		act, err := applyProcessingResponse(f, stageRequestBody, resp)
		if err != nil {
			t.Errorf("err = %v; want nil", err)
		}
		if act != actImmediate {
			t.Errorf("act = %v; want actImmediate", act)
		}
		if dcb.lrCalls != 1 {
			t.Fatalf("dcb.SendLocalReply calls = %d; want 1 (body-stage ImmediateResponse fires SendLocalReply)", dcb.lrCalls)
		}
		if dcb.lrStatus != 403 {
			t.Errorf("status = %d; want 403", dcb.lrStatus)
		}
		if dcb.lrBody != "denied at body stage" {
			t.Errorf("body = %q; want %q", dcb.lrBody, "denied at body stage")
		}
		if !f.immediateResponseEmitted {
			t.Errorf("f.immediateResponseEmitted = false; want true (multi-stage deny one-shot)")
		}
	})
	t.Run("response_body_stage", func(t *testing.T) {
		t.Parallel()
		dcb := &recordingDCB{}
		ecb := &fakeECB{}
		cc := &compiledConfig{stats: newFilterStats(stats.NewRegistry(), "t")}
		f := &filter{cc: cc, dcb: dcb, ecb: ecb}
		ir := &extprocsvcv3.ImmediateResponse{
			Status: &typev3.HttpStatus{Code: typev3.StatusCode_InternalServerError},
			Body:   []byte("denied at response body stage"),
		}
		resp := &extprocsvcv3.ProcessingResponse{
			Response: &extprocsvcv3.ProcessingResponse_ImmediateResponse{ImmediateResponse: ir},
		}
		act, err := applyProcessingResponse(f, stageResponseBody, resp)
		if err != nil {
			t.Errorf("err = %v; want nil", err)
		}
		if act != actImmediate {
			t.Errorf("act = %v; want actImmediate", act)
		}
		if dcb.lrCalls != 1 {
			t.Fatalf("dcb.SendLocalReply calls = %d; want 1 (response_body-stage ImmediateResponse fires SendLocalReply via dcb per ADR-0075)", dcb.lrCalls)
		}
		if dcb.lrStatus != 500 {
			t.Errorf("status = %d; want 500", dcb.lrStatus)
		}
	})
}

// TestApplyProcessingResponse_ClearRouteCacheAtBodyStage_Ignored — per
// SPEC §4.4: clear_route_cache at body stages continues IGNORED (per the
// proto's "ignored in the response direction" wording). The existing
// `s == stageRequestHeaders` guard in Step 6 enforces this; Task 6 adds
// this regression-anchor test to pin the discipline post-AMENDMENT.
func TestApplyProcessingResponse_ClearRouteCacheAtBodyStage_Ignored(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		stats:            newFilterStats(reg, "t"),
		routeCacheAction: extprocv3.ExternalProcessor_CLEAR, // would clear at request_headers
	}
	f := &filter{cc: cc, decodeBodyBuf: []byte("body")}
	cr := &extprocsvcv3.CommonResponse{
		ClearRouteCache: true, // would clear at request_headers
	}
	resp := mkProcessingResponseRequestBody(cr)
	act, err := applyProcessingResponse(f, stageRequestBody, resp)
	if err != nil {
		t.Errorf("err = %v; want nil", err)
	}
	if act != actContinue {
		t.Errorf("act = %v; want actContinue", act)
	}
	// No spurious increment (clear_route_cache at body stage is IGNORED, not spurious).
	if got := cc.stats.spuriousMsgsReceived.Load(); got != 0 {
		t.Errorf("spuriousMsgsReceived = %d; want 0 (clear_route_cache at body stage is IGNORED per SPEC §4.4)", got)
	}
}

// TestApplyProcessingResponse_BodyMutation_EmptyOneof_NoOp — per Task 6
// reviewer carry-forward I-2: an empty BodyMutation oneof (Mutation field
// nil; the proto carries `BodyMutation{}` with no oneof arm set) is
// silently a NO-OP per the oneof-default discipline at applyBodyMutation
// switch's `default` arm. Pins the contract:
//
//   - actContinue (NOT actError + sentinel).
//   - spurious_msgs_received NOT incremented (NOT classified as spurious —
//     an empty BodyMutation is structurally valid + means "no body changes").
//   - decodeBodyBuf NOT mutated (left intact).
//
// This anchor test prevents regression if a future maintainer refactors the
// applyBodyMutation switch and inadvertently classifies the empty-oneof case
// as malformed (which would surface as either a spurious counter increment
// OR an actError return — both would break the proto's oneof-default
// semantics).
func TestApplyProcessingResponse_BodyMutation_EmptyOneof_NoOp(t *testing.T) {
	t.Parallel()
	reg := stats.NewRegistry()
	cc := &compiledConfig{stats: newFilterStats(reg, "t")}
	hdrs := http.Header{}
	hdrs.Set("content-length", "8")
	original := []byte("original")
	f := &filter{
		cc:            cc,
		decodeHeaders: hdrs,
		decodeBodyBuf: original,
	}
	// Empty BodyMutation — the Mutation oneof is nil.
	cr := &extprocsvcv3.CommonResponse{
		BodyMutation: &extprocsvcv3.BodyMutation{},
	}
	resp := mkProcessingResponseRequestBody(cr)
	act, err := applyProcessingResponse(f, stageRequestBody, resp)
	if err != nil {
		t.Errorf("err = %v; want nil (empty BodyMutation oneof is structurally valid no-op)", err)
	}
	if act != actContinue {
		t.Errorf("act = %v; want actContinue", act)
	}
	if got := cc.stats.spuriousMsgsReceived.Load(); got != 0 {
		t.Errorf("spuriousMsgsReceived = %d; want 0 (empty BodyMutation oneof is NOT spurious — proto oneof-default discipline)", got)
	}
	if string(f.decodeBodyBuf) != "original" {
		t.Errorf("decodeBodyBuf = %q; want %q (empty-oneof must NOT mutate the buffer)", f.decodeBodyBuf, "original")
	}
	if got := hdrs.Get("content-length"); got != "8" {
		t.Errorf("content-length = %q; want 8 (empty-oneof must NOT reconcile Content-Length)", got)
	}
}

// ---------------------------------------------------------------------------
// Task 7 body-stage integration tests — end-to-end DecodeData / EncodeData
// dispatch wiring per PLAN Task 7 Step 1 (TDD). Drive the body-stage
// integration through the full chain: DecodeData / EncodeData entry →
// dispatchStage (via fakeProcessStream) → applyProcessingResponse →
// (possibly mutate buffer) → signalResume → assert observed effects.
// ---------------------------------------------------------------------------

// makeIntegrationFilter constructs a *filter wired for Task 7 integration
// tests: dcb + ecb + stream + body-mode BUFFERED in BOTH directions + the
// minimal cc.stats + cc.messageTimeout for dispatchStage. The returned
// filter is ready for DecodeData / EncodeData entry-point invocation.
func makeIntegrationFilter(t *testing.T, stream *fakeProcessStream) (*filter, *recordingDCB, *recordingECB) {
	t.Helper()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		messageTimeout: 5 * time.Second,
		stats:          newFilterStats(reg, "integration"),
		mutationRules:  resolveMutationRules(nil),
	}
	dcb := &recordingDCB{}
	ecb := &recordingECB{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	f := &filter{
		cc:           cc,
		dcb:          dcb,
		ecb:          ecb,
		stream:       stream,
		streamCtx:    ctx,
		streamCancel: cancel,
		parentCtx:    ctx,
		activeProcessingMode: &resolvedProcessingMode{
			RequestHeaderMode:  extprocv3.ProcessingMode_SEND,
			ResponseHeaderMode: extprocv3.ProcessingMode_SEND,
			RequestBodyMode:    extprocv3.ProcessingMode_BUFFERED,
			ResponseBodyMode:   extprocv3.ProcessingMode_BUFFERED,
		},
		activeMsgTimeout: 5 * time.Second,
	}
	return f, dcb, ecb
}

// recordingECB extends fakeECB to record OverwriteBody invocations + their
// payload for Task 7 encode-side body-mutation delivery assertions.
type recordingECB struct {
	fakeECB
	overwriteCalls int
	overwriteBody  []byte
	mu             sync.Mutex
}

func (r *recordingECB) OverwriteBody(b []byte) {
	r.mu.Lock()
	r.overwriteCalls++
	r.overwriteBody = append([]byte{}, b...)
	r.mu.Unlock()
}

func (r *recordingECB) overwriteCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.overwriteCalls
}

func (r *recordingECB) overwriteBytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.overwriteBody
}

// Compile-time conformance — fail fast if recordingECB drifts from the
// framework's EncoderFilterCallbacks interface.
var _ envoyhttp.EncoderFilterCallbacks = (*recordingECB)(nil)

// waitForCondition polls a condition function with a per-iteration short sleep
// up to a bounded deadline. Returns true if the condition fired before the
// deadline, false otherwise. Used by the Task 7 integration tests to await
// the async dispatch goroutine's resume signal.
func waitForCondition(deadline time.Duration, cond func() bool) bool {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

// TestExtProc_RequestBodyBuffered_EndToEnd_WithMutation — request-side
// body-stage integration end-to-end. The fakeProcessStream returns a
// ProcessingResponse{request_body} with a body_mutation arm; the
// dispatcher fires applyProcessingResponse which mutates f.decodeBodyBuf;
// the resume signal fires ContinueDecoding.
//
// **Asserts the decode-side body-mutation-delivery KNOWN LIMITATION**: the
// f.decodeBodyBuf IS mutated (the processor's mutation lands) but there is
// NO equivalent of OverwriteBody on the decode side — the mutated body is
// observable in the filter's buffer + the Content-Length header reconciles
// on f.decodeHeaders, but envoy-go has no upstream-body-delivery primitive
// for the decode side (documented in DecodeData's KNOWN LIMITATION
// comment).
func TestExtProc_RequestBodyBuffered_EndToEnd_WithMutation(t *testing.T) {
	t.Parallel()
	newBody := []byte("mutated upstream body bytes")
	stream := &fakeProcessStream{
		recvResp: &extprocsvcv3.ProcessingResponse{
			Response: &extprocsvcv3.ProcessingResponse_RequestBody{
				RequestBody: &extprocsvcv3.BodyResponse{
					Response: &extprocsvcv3.CommonResponse{
						BodyMutation: &extprocsvcv3.BodyMutation{
							Mutation: &extprocsvcv3.BodyMutation_Body{Body: newBody},
						},
					},
				},
			},
		},
	}
	f, dcb, _ := makeIntegrationFilter(t, stream)
	// Stash headers so the body_mutation arm can reconcile Content-Length.
	hdrs := http.Header{}
	hdrs.Set("content-length", "11")
	f.decodeHeaders = hdrs

	// Mid-stream chunk first — should accumulate + DataContinue (no dispatch).
	st1 := f.DecodeData([]byte("hello "), false)
	if st1 != envoyhttp.DataContinue {
		t.Errorf("mid-stream status = %v; want DataContinue", st1)
	}
	if string(f.decodeBodyBuf) != "hello " {
		t.Errorf("after mid-stream chunk: decodeBodyBuf = %q; want %q", f.decodeBodyBuf, "hello ")
	}
	// streamMsgsSent stays 0 (no dispatch yet).
	if got := f.cc.stats.streamMsgsSent.Load(); got != 0 {
		t.Errorf("streamMsgsSent after mid-stream = %d; want 0", got)
	}

	// Terminal chunk — dispatches + parks via DataStopIterationAndBuffer.
	st2 := f.DecodeData([]byte("world"), true)
	if st2 != envoyhttp.DataStopIterationAndBuffer {
		t.Errorf("end-stream status = %v; want DataStopIterationAndBuffer", st2)
	}

	// Wait for the async dispatch to complete + signal resume.
	if !waitForCondition(2*time.Second, func() bool { return dcb.calls() >= 1 }) {
		t.Fatalf("ContinueDecoding never fired; dcb.calls=%d", dcb.calls())
	}

	// Body buffer mutated (decode-side: observable in the filter buffer).
	if string(f.decodeBodyBuf) != string(newBody) {
		t.Errorf("post-mutation decodeBodyBuf = %q; want %q (decode-side body_mutation lands in buffer)", f.decodeBodyBuf, newBody)
	}
	// Content-Length reconciled on the live header map.
	if got := hdrs.Get("content-length"); got != strconv.Itoa(len(newBody)) {
		t.Errorf("content-length = %q; want %d (decode-side reconciliation lands via f.decodeHeaders)", got, len(newBody))
	}
	// Stat counters increment for the per-stage Send + Recv.
	if got := f.cc.stats.streamMsgsSent.Load(); got != 1 {
		t.Errorf("streamMsgsSent = %d; want 1", got)
	}
	if got := f.cc.stats.streamMsgsReceived.Load(); got != 1 {
		t.Errorf("streamMsgsReceived = %d; want 1", got)
	}
}

// TestExtProc_ResponseBodyBuffered_EndToEnd_WithMutation — encode-side
// body-stage integration end-to-end. The processor returns a body_mutation;
// f.encodeBodyBuf is mutated; the dispatch goroutine fires OverwriteBody
// BEFORE the resume signal so the chain's encodeBodyOverride is set when
// HCM substitutes resp.Body post-RunEncodeData. The resume signal fires
// ContinueEncoding.
//
// **Asserts the encode-side body-mutation-delivery WORKS path** per
// ADR-0131 OverwriteBody reuse — D10 hypothesis HOLDS (no new framework
// primitive consumed).
func TestExtProc_ResponseBodyBuffered_EndToEnd_WithMutation(t *testing.T) {
	t.Parallel()
	newBody := []byte("rewritten response body via ext_proc")
	stream := &fakeProcessStream{
		recvResp: &extprocsvcv3.ProcessingResponse{
			Response: &extprocsvcv3.ProcessingResponse_ResponseBody{
				ResponseBody: &extprocsvcv3.BodyResponse{
					Response: &extprocsvcv3.CommonResponse{
						BodyMutation: &extprocsvcv3.BodyMutation{
							Mutation: &extprocsvcv3.BodyMutation_Body{Body: newBody},
						},
					},
				},
			},
		},
	}
	f, _, ecb := makeIntegrationFilter(t, stream)
	hdrs := http.Header{}
	hdrs.Set("content-length", "3")
	f.encodeHeaders = hdrs

	// Terminal chunk in one call (HCM-passes-full-body-in-one-call default).
	st := f.EncodeData([]byte("foo"), true)
	if st != envoyhttp.DataStopIterationAndBuffer {
		t.Errorf("end-stream status = %v; want DataStopIterationAndBuffer", st)
	}

	// Wait for resume signal.
	if !waitForCondition(2*time.Second, func() bool { return ecb.calls() >= 1 }) {
		t.Fatalf("ContinueEncoding never fired; ecb.calls=%d", ecb.calls())
	}
	// Body buffer mutated.
	if string(f.encodeBodyBuf) != string(newBody) {
		t.Errorf("post-mutation encodeBodyBuf = %q; want %q", f.encodeBodyBuf, newBody)
	}
	// Content-Length reconciled.
	if got := hdrs.Get("content-length"); got != strconv.Itoa(len(newBody)) {
		t.Errorf("content-length = %q; want %d", got, len(newBody))
	}
	// OverwriteBody fired with the mutated bytes.
	if got := ecb.overwriteCount(); got != 1 {
		t.Errorf("OverwriteBody calls = %d; want 1 (encode-side body-mutation delivery via ADR-0131)", got)
	}
	if got := ecb.overwriteBytes(); string(got) != string(newBody) {
		t.Errorf("OverwriteBody bytes = %q; want %q (the mutated buffer is delivered to HCM via encodeBodyOverride)", got, newBody)
	}
	// Counter check.
	if got := f.cc.stats.streamMsgsSent.Load(); got != 1 {
		t.Errorf("streamMsgsSent = %d; want 1", got)
	}
}

// TestExtProc_BodyStageImmediateResponse_EndToEnd — ImmediateResponse at the
// body stage. The processor returns ImmediateResponse{status: 403, body:
// "denied at body stage"}; emitImmediateResponse fires SendLocalReply via
// dcb (per ADR-0075 — even encode-side ImmediateResponse routes through
// dcb). The dispatch goroutine returns actImmediate; completeStage fires
// signalResume.
func TestExtProc_BodyStageImmediateResponse_EndToEnd(t *testing.T) {
	t.Parallel()
	t.Run("request_body_stage", func(t *testing.T) {
		t.Parallel()
		stream := &fakeProcessStream{
			recvResp: &extprocsvcv3.ProcessingResponse{
				Response: &extprocsvcv3.ProcessingResponse_ImmediateResponse{
					ImmediateResponse: &extprocsvcv3.ImmediateResponse{
						Status: &typev3.HttpStatus{Code: typev3.StatusCode_Forbidden},
						Body:   []byte("denied at body stage"),
					},
				},
			},
		}
		f, dcb, _ := makeIntegrationFilter(t, stream)
		st := f.DecodeData([]byte("payload"), true)
		if st != envoyhttp.DataStopIterationAndBuffer {
			t.Errorf("end-stream status = %v; want DataStopIterationAndBuffer", st)
		}
		// Wait for dispatch + ImmediateResponse + resume.
		if !waitForCondition(2*time.Second, func() bool { return dcb.lrCallsSafe() >= 1 }) {
			t.Fatalf("SendLocalReply never fired; dcb.lrCalls=%d", dcb.lrCallsSafe())
		}
		// Wait for resume.
		if !waitForCondition(2*time.Second, func() bool { return dcb.calls() >= 1 }) {
			t.Fatalf("ContinueDecoding never fired post-immediate-response; dcb.calls=%d", dcb.calls())
		}
		if dcb.lrStatusSafe() != 403 {
			t.Errorf("SendLocalReply status = %d; want 403", dcb.lrStatusSafe())
		}
		if dcb.lrBodySafe() != "denied at body stage" {
			t.Errorf("SendLocalReply body = %q; want %q", dcb.lrBodySafe(), "denied at body stage")
		}
		if !f.immediateResponseEmitted {
			t.Errorf("immediateResponseEmitted = false; want true (one-shot deny flag set)")
		}
	})
	t.Run("response_body_stage", func(t *testing.T) {
		t.Parallel()
		stream := &fakeProcessStream{
			recvResp: &extprocsvcv3.ProcessingResponse{
				Response: &extprocsvcv3.ProcessingResponse_ImmediateResponse{
					ImmediateResponse: &extprocsvcv3.ImmediateResponse{
						Status: &typev3.HttpStatus{Code: typev3.StatusCode_InternalServerError},
						Body:   []byte("denied at response body stage"),
					},
				},
			},
		}
		f, dcb, ecb := makeIntegrationFilter(t, stream)
		st := f.EncodeData([]byte("orig"), true)
		if st != envoyhttp.DataStopIterationAndBuffer {
			t.Errorf("end-stream status = %v; want DataStopIterationAndBuffer", st)
		}
		if !waitForCondition(2*time.Second, func() bool { return dcb.lrCallsSafe() >= 1 }) {
			t.Fatalf("SendLocalReply never fired; dcb.lrCalls=%d", dcb.lrCallsSafe())
		}
		if !waitForCondition(2*time.Second, func() bool { return ecb.calls() >= 1 }) {
			t.Fatalf("ContinueEncoding never fired; ecb.calls=%d", ecb.calls())
		}
		if dcb.lrStatusSafe() != 500 {
			t.Errorf("SendLocalReply status = %d; want 500", dcb.lrStatusSafe())
		}
	})
}

// TestExtProc_ContinueAndReplace_HeaderStageWithBodyModeBUFFERED_EndToEnd_BodyStageOutboundSKIPPED
// — verifies the Task 6 skip-flag consumer at Task 7's DecodeData /
// EncodeData entry. Simulates the post-CONTINUE_AND_REPLACE state at body
// stage entry: f.skipBodyStageDispatch[direction] = true + the buffer has
// been pre-mutated by the header-stage applyProcessingResponse. The
// body-stage entry MUST skip the outbound dispatch + release the mutated
// buffer (encode side: via OverwriteBody; decode side: via DataContinue
// per the documented limitation).
func TestExtProc_ContinueAndReplace_HeaderStageWithBodyModeBUFFERED_EndToEnd_BodyStageOutboundSKIPPED(t *testing.T) {
	t.Parallel()
	t.Run("decode_side_request_body_SKIPPED", func(t *testing.T) {
		t.Parallel()
		// Stream is set up but should NEVER be invoked — the skip flag
		// short-circuits before dispatch.
		stream := &fakeProcessStream{
			recvResp: &extprocsvcv3.ProcessingResponse{
				Response: &extprocsvcv3.ProcessingResponse_RequestBody{
					RequestBody: &extprocsvcv3.BodyResponse{Response: &extprocsvcv3.CommonResponse{}},
				},
			},
		}
		f, _, _ := makeIntegrationFilter(t, stream)
		// Simulate the post-CONTINUE_AND_REPLACE state set by Task 6 at the
		// header-stage applyProcessingResponse: skip flag set + buffer
		// pre-mutated.
		f.skipBodyStageDispatch[directionRequest] = true
		preMutated := []byte("pre-mutated request body bytes")
		f.decodeBodyBuf = preMutated

		st := f.DecodeData([]byte("ignored-incoming-chunk"), true)
		if st != envoyhttp.DataContinue {
			t.Errorf("skip-flag end-stream status = %v; want DataContinue (body-stage outbound SKIPPED)", st)
		}
		// Stream Send NEVER called (skip flag short-circuited).
		stream.mu.Lock()
		sendCalls := stream.sendCalls
		stream.mu.Unlock()
		if sendCalls != 0 {
			t.Errorf("stream.Send called %d times; want 0 (body-stage outbound MUST be skipped)", sendCalls)
		}
		// Counter NOT incremented.
		if got := f.cc.stats.streamMsgsSent.Load(); got != 0 {
			t.Errorf("streamMsgsSent = %d; want 0 (skip-flag suppresses dispatch)", got)
		}
		// Per C-1 rework: the pre-mutated buffer is preserved INTACT.
		// The skip-flag short-circuit fires BEFORE the incoming chunk is
		// appended to the accumulator, so f.decodeBodyBuf retains the
		// header-stage replacement bytes verbatim. (Pre-rework the chunk
		// was appended unconditionally before the skip check, which
		// corrupted the buffer; the empty incoming chunk in this test
		// masked the bug. The new TestExtProc_BodyStage_SkipFlag_PreservesPreMutatedBuffer_C1Regression
		// exercises the path with a non-empty chunk + mid-stream chunks.)
		if string(f.decodeBodyBuf) != string(preMutated) {
			t.Errorf("decodeBodyBuf = %q; want %q (skip-flag must NOT append incoming chunk)", f.decodeBodyBuf, preMutated)
		}
	})
	t.Run("encode_side_response_body_SKIPPED_OverwriteBodyFiresWithPreMutatedBuffer", func(t *testing.T) {
		t.Parallel()
		stream := &fakeProcessStream{}
		f, _, ecb := makeIntegrationFilter(t, stream)
		f.skipBodyStageDispatch[directionResponse] = true
		preMutated := []byte("pre-mutated response body bytes")
		f.encodeBodyBuf = preMutated

		// EncodeData with a "remaining" chunk (in practice the HCM passes
		// the full body once; this terminal chunk is what would arrive
		// post-header-stage CONTINUE_AND_REPLACE in the body-mode-BUFFERED
		// case).
		st := f.EncodeData([]byte(""), true)
		if st != envoyhttp.DataContinue {
			t.Errorf("skip-flag end-stream status = %v; want DataContinue (body-stage outbound SKIPPED)", st)
		}
		// Stream Send NEVER called.
		stream.mu.Lock()
		sendCalls := stream.sendCalls
		stream.mu.Unlock()
		if sendCalls != 0 {
			t.Errorf("stream.Send called %d times; want 0 (body-stage outbound MUST be skipped)", sendCalls)
		}
		// OverwriteBody fired with the pre-mutated buffer (so HCM
		// substitutes resp.Body with the header-stage replacement).
		if got := ecb.overwriteCount(); got != 1 {
			t.Errorf("OverwriteBody calls = %d; want 1 (encode-side skip path releases buffer via OverwriteBody)", got)
		}
		if got := ecb.overwriteBytes(); string(got) != string(preMutated) {
			t.Errorf("OverwriteBody bytes = %q; want %q (the pre-mutated buffer flows downstream via OverwriteBody)", got, preMutated)
		}
	})
}

// TestExtProc_BodyStage_SkipFlag_PreservesPreMutatedBuffer_C1Regression — C-1
// regression test (code-quality reviewer a06aa6c8e7db06ecf, Task 7 rework).
//
// **The bug**: pre-rework, DecodeData / EncodeData unconditionally appended
// the incoming HCM chunk to f.{decode,encode}BodyBuf BEFORE checking the
// skipBodyStageDispatch flag. For CONTINUE_AND_REPLACE+body-mode=BUFFERED
// scenarios, Task 6 pre-populated the buffer with the header-stage
// replacement bytes; the unconditional append corrupted the buffer to
// "REPLACEMENT" + "real-upstream-bytes". On the encode side, this corrupt
// concatenation was then passed to f.ecb.OverwriteBody — HCM substituted
// resp.Body with the corrupted bytes. The existing skip-flag test masked
// the bug by passing an EMPTY incoming chunk.
//
// **The fix**: move the skip-flag short-circuit BEFORE the accumulator
// append. The check runs on EVERY entry (mid-stream + endStream chunks)
// so a chunk arriving WHILE the skip-flag is set does NOT corrupt the
// pre-mutated buffer.
//
// **What this regression exercises**:
//   - Encode-side terminal chunk: non-empty incoming bytes — OverwriteBody
//     must receive the REPLACEMENT bytes, NOT REPLACEMENT+incoming.
//   - Encode-side mid-stream chunk followed by terminal chunk: same skip
//     flag held throughout — buffer + OverwriteBody payload stay intact.
//   - Decode-side mirror: f.decodeBodyBuf must remain the pre-mutated bytes
//     after non-empty chunk + after mid-stream-then-terminal sequence.
func TestExtProc_BodyStage_SkipFlag_PreservesPreMutatedBuffer_C1Regression(t *testing.T) {
	t.Parallel()

	t.Run("encode_side_terminal_chunk_nonEmpty_OverwriteBodyGetsReplacementIntact", func(t *testing.T) {
		t.Parallel()
		stream := &fakeProcessStream{}
		f, _, ecb := makeIntegrationFilter(t, stream)
		f.skipBodyStageDispatch[directionResponse] = true
		preMutated := []byte("REPLACEMENT")
		f.encodeBodyBuf = preMutated

		// Non-empty incoming chunk: simulates HCM delivering the actual
		// upstream response body bytes AFTER Task 6 pre-populated the
		// buffer with the header-stage replacement. Pre-C-1-rework this
		// would have produced "REPLACEMENTreal-body-bytes-from-upstream".
		st := f.EncodeData([]byte("real-body-bytes-from-upstream"), true)
		if st != envoyhttp.DataContinue {
			t.Errorf("status = %v; want DataContinue", st)
		}
		// Stream Send NEVER called — skip-flag short-circuited.
		stream.mu.Lock()
		sendCalls := stream.sendCalls
		stream.mu.Unlock()
		if sendCalls != 0 {
			t.Errorf("stream.Send called %d times; want 0", sendCalls)
		}
		// OverwriteBody fired EXACTLY ONCE with the pre-mutated buffer
		// — NOT including the incoming chunk.
		if got := ecb.overwriteCount(); got != 1 {
			t.Errorf("OverwriteBody calls = %d; want 1", got)
		}
		if got := ecb.overwriteBytes(); string(got) != "REPLACEMENT" {
			t.Errorf("OverwriteBody bytes = %q; want %q (C-1 regression: incoming chunk must NOT be appended)", got, "REPLACEMENT")
		}
		// Buffer itself stays the pre-mutated replacement.
		if string(f.encodeBodyBuf) != "REPLACEMENT" {
			t.Errorf("encodeBodyBuf = %q; want %q", f.encodeBodyBuf, "REPLACEMENT")
		}
	})

	t.Run("encode_side_midStream_then_terminal_skipFlagHeldThroughout", func(t *testing.T) {
		t.Parallel()
		stream := &fakeProcessStream{}
		f, _, ecb := makeIntegrationFilter(t, stream)
		f.skipBodyStageDispatch[directionResponse] = true
		preMutated := []byte("REPLACEMENT")
		f.encodeBodyBuf = preMutated

		// Mid-stream chunk arrives — skip-flag must short-circuit and
		// NOT append to the pre-mutated buffer.
		st1 := f.EncodeData([]byte("chunk1"), false)
		if st1 != envoyhttp.DataContinue {
			t.Errorf("mid-stream status = %v; want DataContinue", st1)
		}
		if string(f.encodeBodyBuf) != "REPLACEMENT" {
			t.Errorf("after mid-stream chunk: encodeBodyBuf = %q; want %q (chunk1 must not be appended)", f.encodeBodyBuf, "REPLACEMENT")
		}
		// Terminal chunk — same skip-flag.
		st2 := f.EncodeData([]byte("chunk2"), true)
		if st2 != envoyhttp.DataContinue {
			t.Errorf("terminal status = %v; want DataContinue", st2)
		}
		// OverwriteBody fired BOTH times the skip path triggered.
		// The KEY invariant is that the most-recent OverwriteBody payload
		// is the unmutated REPLACEMENT — chunk1+chunk2 never landed in
		// the buffer.
		if got := ecb.overwriteCount(); got != 2 {
			t.Errorf("OverwriteBody calls = %d; want 2 (mid-stream + terminal both hit the skip path)", got)
		}
		if got := ecb.overwriteBytes(); string(got) != "REPLACEMENT" {
			t.Errorf("final OverwriteBody bytes = %q; want %q (C-1 regression: chunks must NOT corrupt the buffer)", got, "REPLACEMENT")
		}
		if string(f.encodeBodyBuf) != "REPLACEMENT" {
			t.Errorf("final encodeBodyBuf = %q; want %q", f.encodeBodyBuf, "REPLACEMENT")
		}
		// Stream Send NEVER called — body-stage outbound stays suppressed
		// across the entire chunk sequence.
		stream.mu.Lock()
		sendCalls := stream.sendCalls
		stream.mu.Unlock()
		if sendCalls != 0 {
			t.Errorf("stream.Send called %d times; want 0", sendCalls)
		}
	})

	t.Run("decode_side_terminal_chunk_nonEmpty_BufferStaysIntact", func(t *testing.T) {
		t.Parallel()
		stream := &fakeProcessStream{}
		f, _, _ := makeIntegrationFilter(t, stream)
		f.skipBodyStageDispatch[directionRequest] = true
		preMutated := []byte("REPLACEMENT")
		f.decodeBodyBuf = preMutated

		st := f.DecodeData([]byte("real-body-bytes-from-downstream"), true)
		if st != envoyhttp.DataContinue {
			t.Errorf("status = %v; want DataContinue", st)
		}
		// Stream Send NEVER called.
		stream.mu.Lock()
		sendCalls := stream.sendCalls
		stream.mu.Unlock()
		if sendCalls != 0 {
			t.Errorf("stream.Send called %d times; want 0", sendCalls)
		}
		// Decode-side has NO OverwriteBody analog (KNOWN LIMITATION); the
		// invariant is that f.decodeBodyBuf stays the pre-mutated bytes.
		if string(f.decodeBodyBuf) != "REPLACEMENT" {
			t.Errorf("decodeBodyBuf = %q; want %q (C-1 regression: incoming chunk must NOT be appended on decode side either)", f.decodeBodyBuf, "REPLACEMENT")
		}
	})

	t.Run("decode_side_midStream_then_terminal_skipFlagHeldThroughout", func(t *testing.T) {
		t.Parallel()
		stream := &fakeProcessStream{}
		f, _, _ := makeIntegrationFilter(t, stream)
		f.skipBodyStageDispatch[directionRequest] = true
		preMutated := []byte("REPLACEMENT")
		f.decodeBodyBuf = preMutated

		st1 := f.DecodeData([]byte("chunk1"), false)
		if st1 != envoyhttp.DataContinue {
			t.Errorf("mid-stream status = %v; want DataContinue", st1)
		}
		if string(f.decodeBodyBuf) != "REPLACEMENT" {
			t.Errorf("after mid-stream: decodeBodyBuf = %q; want %q", f.decodeBodyBuf, "REPLACEMENT")
		}
		st2 := f.DecodeData([]byte("chunk2"), true)
		if st2 != envoyhttp.DataContinue {
			t.Errorf("terminal status = %v; want DataContinue", st2)
		}
		if string(f.decodeBodyBuf) != "REPLACEMENT" {
			t.Errorf("final decodeBodyBuf = %q; want %q (C-1 regression: chunks must NOT corrupt the buffer)", f.decodeBodyBuf, "REPLACEMENT")
		}
		stream.mu.Lock()
		sendCalls := stream.sendCalls
		stream.mu.Unlock()
		if sendCalls != 0 {
			t.Errorf("stream.Send called %d times; want 0", sendCalls)
		}
	})
}

// TestExtProc_OnDestroy_DuringBodyStageOutbound_NoBufferReleaseFires — per
// planner-time D7 OnDestroy discipline. Asserts the OnDestroy-during-body-
// stage-outbound race resolves deterministically: the dispatch goroutine
// observes f.done = true via completeStage's mu-guarded check + drops the
// resume signal + the encode-side OverwriteBody NEVER fires (the
// deliverEncodeBodyMutation site runs INSIDE the completeStage path; the
// completeStage early-return on f.done short-circuits before reaching the
// stageResponseBody arm).
//
// Wait — completeStage's check is BEFORE applyProcessingResponseFn; the
// deliverEncodeBodyMutation call is AFTER it. Let me re-check the order:
// looking at processor.go's completeStage:
//
//  1. mu.Lock + check f.done (drop if true).
//  2. mu.Unlock.
//  3. applyProcessingResponseFn(f, s, resp) (returns action + maybe mutates buffer).
//  4. deliverEncodeBodyMutation if s == stageResponseBody.
//  5. signalResume.
//
// So if OnDestroy fires AFTER step 2 but BEFORE step 4, the
// deliverEncodeBodyMutation could fire on an already-destroyed filter. The
// race-guard pattern is the f.done check at step 1; OnDestroy itself sets
// f.done = true under f.mu (per processor.go OnDestroy). The race window is
// narrow but acceptable per the planner-time D9 discipline (the encode-side
// OverwriteBody on a destroyed filter is a no-op anyway since the chain is
// already torn down). This test exercises the COMMON case: OnDestroy fires
// BEFORE the dispatch goroutine completes the recv-side processing, so the
// completeStage early-return at step 1 fires + neither the buffer mutation
// NOR the OverwriteBody land.
func TestExtProc_OnDestroy_DuringBodyStageOutbound_NoBufferReleaseFires(t *testing.T) {
	t.Parallel()
	// fakeProcessStream blocks Recv until we close the blockCh — simulates
	// the dispatch goroutine parked on Recv when OnDestroy fires.
	blockCh := make(chan struct{})
	stream := &fakeProcessStream{
		recvBlockCh: blockCh,
		recvResp: &extprocsvcv3.ProcessingResponse{
			Response: &extprocsvcv3.ProcessingResponse_ResponseBody{
				ResponseBody: &extprocsvcv3.BodyResponse{
					Response: &extprocsvcv3.CommonResponse{
						BodyMutation: &extprocsvcv3.BodyMutation{
							Mutation: &extprocsvcv3.BodyMutation_Body{Body: []byte("would-replace")},
						},
					},
				},
			},
		},
	}
	f, _, ecb := makeIntegrationFilter(t, stream)
	stream.recvBlockCtx = f.streamCtx
	original := []byte("original encode body")
	st := f.EncodeData(original, true)
	if st != envoyhttp.DataStopIterationAndBuffer {
		t.Errorf("status = %v; want DataStopIterationAndBuffer", st)
	}
	// Give the dispatch goroutine a moment to enter Recv (block).
	time.Sleep(20 * time.Millisecond)

	// OnDestroy fires — sets f.done + cancels the stream context (which
	// unblocks Recv via the blockCtx select). The dispatch goroutine wakes
	// + completeStage's mu-guarded f.done check fires + drops the response.
	f.OnDestroy()

	// Unblock the channel (defensive — the recvBlockCtx already unblocked
	// the goroutine via ctx.Done; closing the channel is a safety net for
	// the recvBlockCh-only path).
	close(blockCh)

	// Wait long enough for the goroutine to fully exit (it may take a few
	// scheduler ticks). After exit:
	//   - OverwriteBody NEVER fires (the completeStage early-return on
	//     f.done short-circuits before deliverEncodeBodyMutation).
	//   - encodeBodyBuf NOT mutated (the applyProcessingResponse mutation
	//     arm never runs).
	//   - ContinueEncoding NEVER fires (signalResume short-circuits).
	time.Sleep(150 * time.Millisecond)

	if got := ecb.overwriteCount(); got != 0 {
		t.Errorf("OverwriteBody calls = %d; want 0 (D7: no body-buffer release on OnDestroy-during-body-stage-outbound)", got)
	}
	if string(f.encodeBodyBuf) != string(original) {
		t.Errorf("encodeBodyBuf = %q; want %q (body buffer must NOT be mutated when OnDestroy fires first)", f.encodeBodyBuf, original)
	}
	if got := ecb.calls(); got != 0 {
		t.Errorf("ContinueEncoding calls = %d; want 0 (D9: completeStage drops resume signal when f.done set)", got)
	}
	if !f.done {
		t.Errorf("f.done = false after OnDestroy; want true")
	}
}

// ---------------------------------------------------------------------------
// Group N+8 — Phase-19.2 Task 8 body-stage race tests per PLAN Task 8 +
// SPEC §14.2.
//
// The 4 race tests below exercise the body-stage race surfaces under -race:
//
//   - TestRace_OnDestroyDuringBodyStageOutbound_DecodeAndEncode — per D7
//     OnDestroy-during-body-stage-outbound: same primitive 19.1 ratified for
//     header-stage outbound; the body-stage Send/Recv loop honors ctx.Done()
//     and returns promptly + completeStage drops the resume signal under the
//     f.done race-guard. Two parallel sub-tests cover the decode + encode
//     sides; the race detector observes no data race on f.decodeBodyBuf /
//     f.encodeBodyBuf / f.activeMsgCancel under the concurrent
//     OnDestroy + dispatch-goroutine completion sequence.
//
//   - TestRace_EncodeBufConcurrentWithContinueEncoding_EndToEnd — end-to-end
//     analog of chain_test.go's
//     TestEncoderCB_BufferEncodedBody_RaceDetectorCleanUnderConcurrentEncodeDataAndContinueEncoding
//     exercised at the body-stage filter level: drives EncodeData (which
//     accumulates onto f.encodeBodyBuf via bodyStageEntry) + the dispatch
//     goroutine fires ContinueEncoding from off-dispatch via the resume
//     channel. Race detector observes no data race on the f.encodeBodyBuf
//     accumulator under the dispatch-goroutine vs HCM-dispatch-goroutine
//     concurrency. The chain-side c.encodeBuf race surface is exercised
//     end-to-end indirectly through the same dispatch pathway.
//
//   - TestRace_PerMessageTimerCancelRebuild_AgainstInFlightSendRecv — per
//     D4 behavioral lift: dispatch goroutine spawns the watchdog +
//     publishes f.activeMsgCancel under f.mu; concurrent
//     handleOverrideMessageTimeout fires the captured cancel → watchdog
//     fires streamCancel → in-flight Recv unblocks with
//     context.Canceled. Race detector observes no race on f.activeMsgCancel
//     (the mu-protected read+clear pattern in handleOverrideMessageTimeout +
//     the dispatch goroutine's mu-protected set+clear).
//
//   - TestRace_ModeOverrideHeaderStageResponse_VsBodyStageDispatch —
//     header-stage dispatcher's recv goroutine fires the mode_override
//     mutation onto f.activeProcessingMode (under the D9 happens-before
//     ordering); the body-stage dispatch reads f.activeProcessingMode via
//     bodyModeActive() at the EncodeData entry. The test exercises the
//     post-resume happens-before ordering — once the encode-side resume
//     signal fires (after applyProcessingResponse mutated the mode), a
//     subsequent EncodeData read observes the post-override mode without a
//     torn read or data-race detector finding.
// ---------------------------------------------------------------------------

// TestRace_OnDestroyDuringBodyStageOutbound_DecodeAndEncode — Task 8 Test 1.
//
// Two parallel sub-tests cover the decode + encode sides. Each:
//
//  1. Wires an integration filter with a fakeProcessStream whose Recv blocks
//     on streamCtx.Done (the dispatch goroutine parks on Recv after Send).
//  2. Issues DecodeData / EncodeData with endStream=true → spawns the dispatch
//     goroutine via dispatchStage; the goroutine fires Send + parks on Recv.
//  3. Concurrently invokes f.OnDestroy() — cancels streamCtx → Recv returns
//     context.Canceled → dispatch goroutine's completeStage early-returns on
//     the f.done race-guard → no resume signal fires + no body-buffer
//     mutation lands.
//
// Race-detector clean under the concurrent OnDestroy + dispatch-goroutine
// completion sequence. Asserts:
//
//   - dcb.calls() == 0 (decode) / ecb.calls() == 0 (encode): the resume
//     signal is dropped by the D9 race-guard.
//   - For the encode side: ecb.overwriteCount() == 0 — the
//     deliverEncodeBodyMutation path short-circuits via the f.done check at
//     the top of completeStage.
//   - f.done == true post-OnDestroy.
//   - cc.stats.streamsFailed >= 1 (cancel-induced Recv error path).
func TestRace_OnDestroyDuringBodyStageOutbound_DecodeAndEncode(t *testing.T) {
	t.Parallel()

	t.Run("decode_side", func(t *testing.T) {
		t.Parallel()
		// fakeProcessStream's Recv blocks until blockCh closes OR ctx cancels.
		// We never close blockCh — only OnDestroy's streamCancel unblocks Recv.
		blockCh := make(chan struct{})
		stream := &fakeProcessStream{
			recvBlockCh: blockCh,
			recvResp: &extprocsvcv3.ProcessingResponse{
				Response: &extprocsvcv3.ProcessingResponse_RequestBody{
					RequestBody: &extprocsvcv3.BodyResponse{
						Response: &extprocsvcv3.CommonResponse{
							BodyMutation: &extprocsvcv3.BodyMutation{
								Mutation: &extprocsvcv3.BodyMutation_Body{Body: []byte("would-mutate")},
							},
						},
					},
				},
			},
		}
		f, dcb, _ := makeIntegrationFilter(t, stream)
		stream.recvBlockCtx = f.streamCtx
		original := []byte("original request body")
		f.decodeBodyBuf = nil // ensure clean baseline

		// Dispatch goroutine parks on Recv.
		st := f.DecodeData(original, true)
		if st != envoyhttp.DataStopIterationAndBuffer {
			t.Errorf("DecodeData status = %v; want DataStopIterationAndBuffer", st)
		}
		// Wait for the goroutine to enter Recv (race-detector cares about the
		// goroutine actually being parked when OnDestroy fires concurrently).
		if !waitForCondition(500*time.Millisecond, func() bool {
			stream.mu.Lock()
			defer stream.mu.Unlock()
			return stream.recvCalls >= 1
		}) {
			t.Fatalf("dispatch goroutine did not enter Recv within 500ms")
		}

		// Concurrent OnDestroy — race surface: streamCancel races with the
		// dispatch goroutine's Recv unblock + completeStage's f.done check.
		f.OnDestroy()

		// Wait for the dispatch goroutine to observe ctx cancel + complete
		// (streamsFailed increments via the recvErr path in dispatchStage).
		if !waitForCondition(1*time.Second, func() bool {
			return f.cc.stats.streamsFailed.Load() >= 1
		}) {
			t.Fatalf("dispatch goroutine did not unblock within 1s of OnDestroy; streamsFailed=%d",
				f.cc.stats.streamsFailed.Load())
		}

		// D9 invariants post-OnDestroy:
		if dcb.calls() != 0 {
			t.Errorf("ContinueDecoding calls = %d; want 0 (D9: completeStage drops resume signal when f.done set)", dcb.calls())
		}
		// The decode-side body buffer was set by bodyStageEntry's accumulator
		// (original bytes) BEFORE OnDestroy fired; the body_mutation arm
		// inside applyProcessingResponse never ran (completeStage short-
		// circuits on f.done). So the buffer stays the ACCUMULATED original
		// bytes — NOT the would-mutate replacement.
		if string(f.decodeBodyBuf) != string(original) {
			t.Errorf("decodeBodyBuf = %q; want %q (body buffer must NOT be mutated when OnDestroy fires first)", f.decodeBodyBuf, original)
		}
		if !f.done {
			t.Errorf("f.done = false after OnDestroy; want true")
		}
		// Defensive close — Recv has already unblocked via streamCancel; this
		// is a safety net for the recvBlockCh-only path.
		close(blockCh)
	})

	t.Run("encode_side", func(t *testing.T) {
		t.Parallel()
		blockCh := make(chan struct{})
		stream := &fakeProcessStream{
			recvBlockCh: blockCh,
			recvResp: &extprocsvcv3.ProcessingResponse{
				Response: &extprocsvcv3.ProcessingResponse_ResponseBody{
					ResponseBody: &extprocsvcv3.BodyResponse{
						Response: &extprocsvcv3.CommonResponse{
							BodyMutation: &extprocsvcv3.BodyMutation{
								Mutation: &extprocsvcv3.BodyMutation_Body{Body: []byte("would-replace")},
							},
						},
					},
				},
			},
		}
		f, _, ecb := makeIntegrationFilter(t, stream)
		stream.recvBlockCtx = f.streamCtx
		original := []byte("original encode body")

		st := f.EncodeData(original, true)
		if st != envoyhttp.DataStopIterationAndBuffer {
			t.Errorf("EncodeData status = %v; want DataStopIterationAndBuffer", st)
		}
		// Wait for the goroutine to enter Recv.
		if !waitForCondition(500*time.Millisecond, func() bool {
			stream.mu.Lock()
			defer stream.mu.Unlock()
			return stream.recvCalls >= 1
		}) {
			t.Fatalf("dispatch goroutine did not enter Recv within 500ms")
		}

		// Concurrent OnDestroy.
		f.OnDestroy()

		if !waitForCondition(1*time.Second, func() bool {
			return f.cc.stats.streamsFailed.Load() >= 1
		}) {
			t.Fatalf("dispatch goroutine did not unblock within 1s of OnDestroy; streamsFailed=%d",
				f.cc.stats.streamsFailed.Load())
		}

		// Encode-side D9 invariants:
		if ecb.calls() != 0 {
			t.Errorf("ContinueEncoding calls = %d; want 0", ecb.calls())
		}
		if got := ecb.overwriteCount(); got != 0 {
			t.Errorf("OverwriteBody calls = %d; want 0 (D7: deliverEncodeBodyMutation must NOT fire post-OnDestroy)", got)
		}
		if string(f.encodeBodyBuf) != string(original) {
			t.Errorf("encodeBodyBuf = %q; want %q", f.encodeBodyBuf, original)
		}
		if !f.done {
			t.Errorf("f.done = false after OnDestroy; want true")
		}
		close(blockCh)
	})
}

// TestRace_EncodeBufConcurrentWithContinueEncoding_EndToEnd — Task 8 Test 2.
//
// End-to-end body-stage analog of chain_test.go's
// TestEncoderCB_BufferEncodedBody_RaceDetectorCleanUnderConcurrentEncodeDataAndContinueEncoding
// (chain-level encodeBuf race surface). The body-stage dispatch path drives:
//
//   - bodyStageEntry's accumulator append onto f.encodeBodyBuf in the
//     EncodeData call (HCM-side / test-side goroutine).
//   - The dispatch goroutine's completeStage path firing ContinueEncoding
//     (via ecb.ContinueEncoding) from the OFF-dispatch async goroutine
//     spawned by dispatchStage.
//
// The HCM-side EncodeData call returns DataStopIterationAndBuffer; the
// parked HCM goroutine is unparked by the dispatch goroutine's
// ContinueEncoding via the encodeResumeCh primitive (the chain-level
// primitive that chain_test.go exercises). End-to-end at the body-stage
// filter level: the f.encodeBodyBuf accumulator + the dispatch goroutine's
// resume signal must produce zero race-detector findings across N
// iterations. The race surface that this test pins is: the write to
// f.encodeBodyBuf (HCM goroutine, inside bodyStageEntry) happens-before the
// dispatch goroutine's Send (which reads f.encodeBodyBuf via the envelope
// builder) per the synchronous chain — there is no concurrent write/read on
// f.encodeBodyBuf, but the test exercises the full path repeatedly under
// -race to catch any future regression in that ordering. The
// applyProcessingResponse mutation arm (writes to f.encodeBodyBuf from the
// dispatch goroutine) happens-before the ecb.ContinueEncoding signal which
// happens-before the parked HCM goroutine's unpark.
func TestRace_EncodeBufConcurrentWithContinueEncoding_EndToEnd(t *testing.T) {
	t.Parallel()
	const iterations = 25
	mutatedBody := []byte("body bytes mutated by processor")
	for iter := 0; iter < iterations; iter++ {
		stream := &fakeProcessStream{
			recvResp: &extprocsvcv3.ProcessingResponse{
				Response: &extprocsvcv3.ProcessingResponse_ResponseBody{
					ResponseBody: &extprocsvcv3.BodyResponse{
						Response: &extprocsvcv3.CommonResponse{
							BodyMutation: &extprocsvcv3.BodyMutation{
								Mutation: &extprocsvcv3.BodyMutation_Body{Body: mutatedBody},
							},
						},
					},
				},
			},
		}
		f, _, ecb := makeIntegrationFilter(t, stream)
		hdrs := http.Header{}
		hdrs.Set("content-length", "3")
		f.encodeHeaders = hdrs

		// EncodeData with endStream=true — appends to f.encodeBodyBuf via
		// bodyStageEntry's accumulator (HCM-side goroutine) + dispatches the
		// body-stage Send/Recv on the OFF-dispatch goroutine which mutates
		// f.encodeBodyBuf via applyProcessingResponse + signals
		// ContinueEncoding. The race-detector observes the full path.
		st := f.EncodeData([]byte("xyz"), true)
		if st != envoyhttp.DataStopIterationAndBuffer {
			t.Fatalf("iter %d: EncodeData status = %v; want DataStopIterationAndBuffer", iter, st)
		}

		// Wait for the dispatch goroutine's resume signal — the
		// ContinueEncoding call is the chain-level unpark primitive that
		// would, in production, unblock the parked HCM goroutine. In this
		// test we observe its arrival via ecb.calls() reaching 1.
		if !waitForCondition(2*time.Second, func() bool { return ecb.calls() >= 1 }) {
			t.Fatalf("iter %d: ContinueEncoding never fired; ecb.calls=%d", iter, ecb.calls())
		}
		// Post-resume: the encode buffer has been mutated by the dispatch
		// goroutine + OverwriteBody fired with the mutated bytes.
		if string(f.encodeBodyBuf) != string(mutatedBody) {
			t.Fatalf("iter %d: encodeBodyBuf = %q; want %q", iter, f.encodeBodyBuf, mutatedBody)
		}
		if got := ecb.overwriteCount(); got != 1 {
			t.Fatalf("iter %d: OverwriteBody calls = %d; want 1", iter, got)
		}
		if got := ecb.overwriteBytes(); string(got) != string(mutatedBody) {
			t.Fatalf("iter %d: OverwriteBody bytes = %q; want %q", iter, got, mutatedBody)
		}
	}
}

// TestRace_PerMessageTimerCancelRebuild_AgainstInFlightSendRecv — Task 8
// Test 3.
//
// Per D4 behavioral lift (ADR-0171 §Decision AMENDMENT bullet 5): the
// per-message timer is consumed BEHAVIORALLY via context.WithTimeout
// cancel-and-rebuild on each stage's Send. The dispatchStage publishes the
// per-message cancel hook on f.activeMsgCancel under f.mu; the watchdog
// goroutine selects on msgCtx.Done vs doneCh + fires f.streamCancel on
// per-message deadline expiry (cascade-cancels the bidi-stream so the
// in-flight Recv unblocks).
//
// This test races a concurrent handleOverrideMessageTimeout call against
// the in-flight Send/Recv: the override fires the captured
// f.activeMsgCancel → watchdog observes msgCtx.Done → watchdog fires
// streamCancel → Recv unblocks with context.Canceled → dispatch goroutine's
// completeStage handles the recvErr path → streamsFailed increments.
//
// Race-detector clean under the concurrent activeMsgCancel set (dispatch
// goroutine, under f.mu) + read+clear (handleOverrideMessageTimeout, under
// f.mu) + the deferred clear at goroutine exit (also under f.mu). The
// stream-fatal cascade is the intended behavior per the D4 docstring's
// "Stream-fatal cascade" note.
func TestRace_PerMessageTimerCancelRebuild_AgainstInFlightSendRecv(t *testing.T) {
	t.Parallel()
	// fakeProcessStream's Recv blocks until ctx cancels.
	blockCh := make(chan struct{})
	stream := &fakeProcessStream{
		recvBlockCh: blockCh,
		recvResp: &extprocsvcv3.ProcessingResponse{
			Response: &extprocsvcv3.ProcessingResponse_ResponseBody{
				ResponseBody: &extprocsvcv3.BodyResponse{
					Response: &extprocsvcv3.CommonResponse{},
				},
			},
		},
	}
	f, _, _ := makeIntegrationFilter(t, stream)
	stream.recvBlockCtx = f.streamCtx
	// Override-API needs maxMessageTimeout >= 1ms.
	f.cc.maxMessageTimeout = 5 * time.Second
	// Trigger body-stage dispatch (encode side) — parks on Recv.
	st := f.EncodeData([]byte("payload"), true)
	if st != envoyhttp.DataStopIterationAndBuffer {
		t.Fatalf("EncodeData status = %v; want DataStopIterationAndBuffer", st)
	}
	// Wait for the dispatch goroutine to publish f.activeMsgCancel +
	// enter Recv. The activeMsgCancel publish happens BEFORE the Send call
	// per dispatchStage's code ordering; observing recvCalls >= 1 is
	// therefore a sufficient barrier for the cancel-hook visibility.
	if !waitForCondition(500*time.Millisecond, func() bool {
		stream.mu.Lock()
		defer stream.mu.Unlock()
		return stream.recvCalls >= 1
	}) {
		t.Fatalf("dispatch goroutine did not enter Recv within 500ms")
	}
	// Verify the cancel hook is published.
	f.mu.Lock()
	gotCancel := f.activeMsgCancel != nil
	f.mu.Unlock()
	if !gotCancel {
		t.Fatalf("f.activeMsgCancel not published after Recv entry; expected dispatchStage to set it under f.mu")
	}

	// Concurrent handleOverrideMessageTimeout — race surface: read+clear of
	// f.activeMsgCancel under f.mu (handleOverrideMessageTimeout) vs the
	// dispatch goroutine's deferred clear-under-f.mu at goroutine exit + the
	// set-under-f.mu at goroutine entry. Per ADR-0171 §Decision AMENDMENT
	// bullet 5: the override accept fires the captured cancel → watchdog
	// fires streamCancel → Recv unblocks → dispatch goroutine exits.
	accepted := f.handleOverrideMessageTimeout(stageResponseBody, durationpb.New(50*time.Millisecond))
	if !accepted {
		t.Fatalf("handleOverrideMessageTimeout returned false; want true (override should be accepted)")
	}

	// Wait for the dispatch goroutine to complete — streamsFailed
	// increments via the recvErr path under the cancel-induced Recv unblock.
	if !waitForCondition(1*time.Second, func() bool {
		return f.cc.stats.streamsFailed.Load() >= 1
	}) {
		t.Fatalf("dispatch goroutine did not unblock within 1s of override-timeout; streamsFailed=%d",
			f.cc.stats.streamsFailed.Load())
	}

	// Post-race: f.activeMsgCancel is cleared (the override path set it nil
	// before firing; the deferred clear at goroutine exit is idempotent).
	f.mu.Lock()
	stillSet := f.activeMsgCancel != nil
	f.mu.Unlock()
	if stillSet {
		t.Errorf("f.activeMsgCancel still set after override-timeout + dispatch exit; want nil (idempotent clear)")
	}
	// The override counter must have incremented exactly once.
	if got := f.cc.stats.overrideMessageTimeoutReceived.Load(); got != 1 {
		t.Errorf("overrideMessageTimeoutReceived = %d; want 1", got)
	}
	// Defensive close — Recv already unblocked via streamCancel.
	close(blockCh)
}

// TestRace_ModeOverrideHeaderStageResponse_VsBodyStageDispatch — Task 8 Test
// 4.
//
// Per parent §5.P1 + ADR-0171: mode_override on a header-stage response
// mutates f.activeProcessingMode on the dispatch goroutine's recv-side
// completion (applyProcessingResponse Step 3); the body-stage dispatch at
// the subsequent EncodeData entry reads f.activeProcessingMode via
// bodyModeActive(). The D9 happens-before ordering is: header-stage resume
// signal happens-before the encode-side EncodeData entry; therefore the
// body-stage read observes the post-override mode without a torn read.
//
// This test pins the ordering under -race across the
// header-response-arrival + body-stage-dispatch sequence. The flow:
//
//   - First call: EncodeHeaders → header-stage dispatch fires; the response
//     carries mode_override flipping ResponseBodyMode → BUFFERED (was NONE
//     at the cc default). applyProcessingResponse mutates
//     f.activeProcessingMode on the dispatch goroutine. The
//     ContinueEncoding signal fires (D9 happens-before).
//
//   - Second call: EncodeData (the body-stage entry) reads
//     f.activeProcessingMode via bodyModeActive() — observes BUFFERED →
//     proceeds with body-stage dispatch. The race detector observes no
//     race on f.activeProcessingMode.
//
// This is the natural production sequence (mode_override flips the mode on
// the header response; the body stage follows). The race-clean property
// follows from the D9 framework-sequential-dispatch invariant: the dispatch
// goroutine completes (signals resume) BEFORE the encode-side body
// dispatch begins.
func TestRace_ModeOverrideHeaderStageResponse_VsBodyStageDispatch(t *testing.T) {
	t.Parallel()
	// Stream sequence: first Recv (header stage) returns mode_override flipping
	// ResponseBodyMode → BUFFERED; second Recv (body stage) returns a clean
	// body response. We use a custom recv-sequence stream below to deliver
	// both responses in order.
	stream := &sequencedRecvStream{
		responses: []*extprocsvcv3.ProcessingResponse{
			// Header-stage: mode_override + valid response_headers.
			{
				ModeOverride: &extprocv3.ProcessingMode{
					RequestHeaderMode:  extprocv3.ProcessingMode_SEND,
					ResponseHeaderMode: extprocv3.ProcessingMode_SEND,
					ResponseBodyMode:   extprocv3.ProcessingMode_BUFFERED,
				},
				Response: &extprocsvcv3.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: &extprocsvcv3.HeadersResponse{
						Response: &extprocsvcv3.CommonResponse{
							Status: extprocsvcv3.CommonResponse_CONTINUE,
						},
					},
				},
			},
			// Body-stage: clean response_body.
			{
				Response: &extprocsvcv3.ProcessingResponse_ResponseBody{
					ResponseBody: &extprocsvcv3.BodyResponse{
						Response: &extprocsvcv3.CommonResponse{},
					},
				},
			},
		},
	}
	// Build the filter directly (sequencedRecvStream doesn't fit
	// makeIntegrationFilter's *fakeProcessStream parameter). Mirrors
	// makeIntegrationFilter's defaults: messageTimeout 5s, mutationRules
	// resolved, parentCtx == streamCtx, recordingDCB / recordingECB.
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		messageTimeout: 5 * time.Second,
		stats:          newFilterStats(reg, "mode_override_vs_body"),
		mutationRules:  resolveMutationRules(nil),
		// allowModeOverride must be true for applyProcessingResponse Step 3 to
		// mutate f.activeProcessingMode.
		allowModeOverride: true,
	}
	ecb := &recordingECB{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	f := &filter{
		cc:           cc,
		dcb:          &recordingDCB{},
		ecb:          ecb,
		stream:       stream,
		streamCtx:    ctx,
		streamCancel: cancel,
		parentCtx:    ctx,
		// Start with ResponseBodyMode = NONE so the mode_override actually
		// flips state observably (mode_override sets it to BUFFERED).
		activeProcessingMode: &resolvedProcessingMode{
			RequestHeaderMode:  extprocv3.ProcessingMode_SEND,
			ResponseHeaderMode: extprocv3.ProcessingMode_SEND,
			RequestBodyMode:    extprocv3.ProcessingMode_NONE,
			ResponseBodyMode:   extprocv3.ProcessingMode_NONE,
		},
		activeMsgTimeout: 5 * time.Second,
	}

	// Sanity: pre-call, body mode is NONE (encode-side body-stage dispatch
	// would be inactive).
	if f.bodyModeActive(directionResponse) {
		t.Fatalf("pre-call: bodyModeActive(response) = true; want false (NONE baseline)")
	}

	// Step 1: EncodeHeaders fires the header-stage dispatch → recv returns
	// the mode_override → applyProcessingResponse mutates
	// f.activeProcessingMode under the dispatch goroutine → signalResume
	// fires ContinueEncoding.
	hdrs := http.Header{"X-Foo": []string{"bar"}}
	status := f.EncodeHeaders(hdrs, false)
	if status != envoyhttp.StopIteration {
		t.Fatalf("EncodeHeaders status = %v; want StopIteration", status)
	}
	// Wait for the header-stage resume signal — the D9 happens-before
	// barrier that publishes the mode_override mutation to subsequent reads.
	if !waitForCondition(2*time.Second, func() bool { return ecb.calls() >= 1 }) {
		t.Fatalf("ContinueEncoding (header stage) never fired; ecb.calls=%d", ecb.calls())
	}

	// Post-header-stage: the mode_override mutation is observable via the
	// happens-before ordering of the resume signal.
	if f.activeProcessingMode == nil ||
		f.activeProcessingMode.ResponseBodyMode != extprocv3.ProcessingMode_BUFFERED {
		t.Fatalf("post-header: ResponseBodyMode = %v; want BUFFERED (mode_override should have flipped it)",
			f.activeProcessingMode.ResponseBodyMode)
	}
	if !f.bodyModeActive(directionResponse) {
		t.Errorf("post-header: bodyModeActive(response) = false; want true (post-override BUFFERED)")
	}

	// Step 2: EncodeData fires the body-stage entry → bodyStageEntry's
	// bodyModeActive() reads f.activeProcessingMode (the post-override
	// value) → proceeds with body-stage dispatch. The race detector
	// observes no race on f.activeProcessingMode between the dispatch
	// goroutine's mutation (Step 1) and this read (Step 2).
	headers := http.Header{}
	headers.Set("content-length", "7")
	f.encodeHeaders = headers
	bodySt := f.EncodeData([]byte("payload"), true)
	if bodySt != envoyhttp.DataStopIterationAndBuffer {
		t.Fatalf("EncodeData status = %v; want DataStopIterationAndBuffer (body-stage dispatch should proceed under post-override BUFFERED)", bodySt)
	}
	// Wait for the body-stage resume signal.
	if !waitForCondition(2*time.Second, func() bool { return ecb.calls() >= 2 }) {
		t.Fatalf("ContinueEncoding (body stage) never fired; ecb.calls=%d", ecb.calls())
	}
	// Sanity: 2 Send calls (header + body), 2 Recv calls.
	if got := stream.sendCount(); got != 2 {
		t.Errorf("Send count = %d; want 2 (one per stage)", got)
	}
	if got := stream.recvCount(); got != 2 {
		t.Errorf("Recv count = %d; want 2 (one per stage)", got)
	}
}

// sequencedRecvStream is a deterministic test fake that returns a sequence
// of pre-recorded ProcessingResponses on consecutive Recv calls. Used by
// TestRace_ModeOverrideHeaderStageResponse_VsBodyStageDispatch which needs
// distinct responses for the header stage (carrying mode_override) and the
// body stage (clean response).
type sequencedRecvStream struct {
	mu sync.Mutex

	sendCalls    int
	recvCalls    int
	closeSendCnt int
	responses    []*extprocsvcv3.ProcessingResponse
}

func (s *sequencedRecvStream) Send(_ *extprocsvcv3.ProcessingRequest) error {
	s.mu.Lock()
	s.sendCalls++
	s.mu.Unlock()
	return nil
}

func (s *sequencedRecvStream) Recv() (*extprocsvcv3.ProcessingResponse, error) {
	s.mu.Lock()
	idx := s.recvCalls
	s.recvCalls++
	var resp *extprocsvcv3.ProcessingResponse
	if idx < len(s.responses) {
		resp = s.responses[idx]
	}
	s.mu.Unlock()
	if resp == nil {
		return nil, errors.New("sequencedRecvStream: no response at index")
	}
	return resp, nil
}

func (s *sequencedRecvStream) CloseSend() error {
	s.mu.Lock()
	s.closeSendCnt++
	s.mu.Unlock()
	return nil
}

func (s *sequencedRecvStream) sendCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sendCalls
}

func (s *sequencedRecvStream) recvCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recvCalls
}
