package extproc

// json.go — ADR-0170 filter-local protojson codec for the http_service-mode
// dispatch path of envoy.filters.http.ext_proc per phase 19.1.
//
// Public surface (private to the `extproc` package per SPEC §3.2):
//
//	marshalProcessingRequest(req *ProcessingRequest) ([]byte, error)
//	unmarshalProcessingResponse(data []byte) (*ProcessingResponse, error)
//
// The codec wraps `google.golang.org/protobuf/encoding/protojson` with the
// MarshalOptions/UnmarshalOptions pinned by ADR-0170 §Decision (per the
// parent phase-19 SPEC §5.P8 hypothesis):
//
//   - MarshalOptions{UseProtoNames: true, EmitUnpopulated: false,
//     UseEnumNumbers: false} — snake_case proto field names, zero-valued
//     fields omitted, enum values rendered as strings. Hypothesized to match
//     reference Envoy's encoding; RATIFIED-PENDING-IMPL-TIME at Task 13
//     fixture-harness scrape vs reference Envoy v1.37.2.
//
//   - UnmarshalOptions{DiscardUnknown: true} — forward-compat tolerance for
//     proto extensions added in future Envoy versions; load-bearing for a
//     non-fail-loud parse of newer-Envoy-vintage ProcessingResponses.
//
// **Call sites.** The codec is invoked from `check.go`'s http_service-mode
// path (lands at Task 8): `marshalProcessingRequest` produces the per-stage
// POST body; `unmarshalProcessingResponse` parses the response body. The
// rest of the per-stage dispatch is mode-agnostic (the converged
// `applyProcessingResponse` value path; SPEC §6.7).
//
// **Failure discipline (D8 fail-loud).** On unmarshal failure, the
// dispatcher classifies the error as `streamsFailed++` + dispError (per
// SPEC §4 error-disposition table); the filter then applies the
// `failure_mode_allow` posture per parent §5.P11. The codec itself does
// NOT increment counters or emit log lines — it returns the raw protojson
// error wrapped with a stable prefix for grep-discoverability at the
// dispatcher boundary.
//
// **Direction asymmetry per Pattern A (PLAN Task 6).** Production code
// exposes the REQUEST-direction marshal + the RESPONSE-direction unmarshal
// — the two directions the http_service-mode dispatch actually exercises
// (Envoy POSTs ProcessingRequest, processor responds with ProcessingResponse).
// No peer `unmarshalProcessingRequest` / `marshalProcessingResponse` is
// authored — the inverse directions are not exercised in production. Tests
// that need to verify round-trip semantics call protojson.Unmarshal directly
// on the marshaled bytes per the Task 6 Pattern A test convention (treats
// the production codec as black-box from the test perspective).
//
// **Filter-local at 19.1 per ADR-0170 §Consequences.** The codec is NOT
// generalized into a shared `internal/jsoncodec/` package — no second
// in-tree consumer of protojson-over-HTTP exists at 19.1. Per the
// phase-18.1 ADR-0159 (b)-disposition rationale, generalization is
// reconsidered at the THIRD consumer trigger. The IMPL-time AMENDMENT
// path at parent §5.P8 RATIFIED-PENDING closure (if Envoy diverges from
// `protojson` defaults at the wire-shape scrape) AMENDS ADR-0170 §Decision
// in-place at Task 13 per ADR-0044.

import (
	"errors"
	"fmt"

	extprocsvcv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/protobuf/encoding/protojson"
)

// marshalOpts pins the protojson MarshalOptions per ADR-0170 §Decision.
// Package-level so the per-call dispatch does not re-allocate the
// options struct on each invocation (the struct is small but the
// per-stage path is hot — one marshal per stage per HTTP transaction
// in http_service mode).
//
//   - UseProtoNames: true — render the proto field names as snake_case
//     (e.g., `request_headers`), NOT the proto3 JSON canonical
//     lowerCamelCase (`requestHeaders`). Hypothesized to match reference
//     Envoy's encoding per parent §5.P8; the IMPL fixture-harness scrape
//     at Task 13 closes the pin definitively (if reference Envoy emits
//     lowerCamelCase, this field flips per ADR-0044 in-place AMENDMENT).
//
//   - EmitUnpopulated: false — omit zero-valued fields entirely (the
//     protojson default). Reduces wire payload + matches reference
//     Envoy's omission discipline (hypothesized; closes at Task 13).
//
//   - UseEnumNumbers: false — render enum values as their string names
//     (e.g., `"CONTINUE"`, `"OVERWRITE_IF_EXISTS_OR_ADD"`) rather than
//     their integer ordinals. Standard for human-readable envoy admin
//     pages and processor-side observability.
var marshalOpts = protojson.MarshalOptions{
	UseProtoNames:   true,
	EmitUnpopulated: false,
	UseEnumNumbers:  false,
}

// unmarshalOpts pins the protojson UnmarshalOptions per ADR-0170 §Decision.
// Package-level for the same hot-path-allocation rationale as marshalOpts.
//
//   - DiscardUnknown: true — silently ignore unknown JSON fields rather
//     than failing the parse. Load-bearing for forward-compat: future
//     Envoy proto extensions (new fields added to ProcessingResponse in
//     a newer go-control-plane vintage) land as silently-ignored unknown
//     fields rather than parse failures that would crash every running
//     envoy-go binary against a newer processor. The trade-off is
//     accepting a typo'd processor-side field name as silently ignored;
//     this is acceptable because the processor is the source-of-truth
//     for the response shape (envoy-go is the consumer).
//
//     Contrast with `internal/bootstrap/bootstrap.go:156` which uses
//     `DiscardUnknown: false` — that path parses operator-supplied
//     bootstrap YAML where typo'd field names SHOULD fail loudly to
//     surface misconfigurations at boot. The ext_proc dispatch path
//     parses processor-emitted JSON in production, where the discipline
//     inverts: be tolerant of unknown fields to survive proto-version
//     skew across the envoy-go ↔ processor boundary.
var unmarshalOpts = protojson.UnmarshalOptions{
	DiscardUnknown: true,
}

// marshalProcessingRequest serializes a *ProcessingRequest to JSON bytes
// using the ADR-0170 §Decision MarshalOptions.
//
// **Contract:**
//   - Returns the JSON-encoded bytes + nil on success.
//   - Returns nil + non-nil error on:
//   - nil input (defensive — the per-stage dispatcher at Task 8 never
//     passes nil in production, but the codec is defensive at the API
//     boundary).
//   - any protojson.Marshal failure (extremely rare for well-formed
//     proto values — proto values are statically valid by construction;
//     marshal-failure typically indicates an exotic well-known-type
//     encoding issue).
//
// **Concurrent-safe.** The package-level `marshalOpts` is read-only;
// protojson.Marshal is goroutine-safe. The caller may invoke
// marshalProcessingRequest concurrently from any goroutine.
//
// **Error wrapping.** Wraps the underlying protojson error with the
// stable prefix `"extproc: marshal ProcessingRequest: "` for
// grep-discoverability at log-scrape time. The dispatcher at Task 8
// further wraps with stage context (e.g., the stage discriminator name).
func marshalProcessingRequest(req *extprocsvcv3.ProcessingRequest) ([]byte, error) {
	if req == nil {
		return nil, errors.New("extproc: marshal ProcessingRequest: nil input")
	}
	data, err := marshalOpts.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("extproc: marshal ProcessingRequest: %w", err)
	}
	return data, nil
}

// unmarshalProcessingResponse deserializes JSON bytes into a
// *ProcessingResponse using the ADR-0170 §Decision UnmarshalOptions
// (DiscardUnknown: true).
//
// **Contract:**
//   - Returns a freshly-allocated *ProcessingResponse + nil on success.
//   - Returns nil + non-nil error on any protojson.Unmarshal failure
//     — empty input, truncated JSON, syntactically-invalid JSON,
//     type-mismatched JSON (e.g., a string where a proto message is
//     expected). Unknown fields are NOT errors per DiscardUnknown:true.
//
// **D8 fail-loud discipline.** A parse failure here is the load-bearing
// signal for the dispatcher (Task 8) to classify the response as
// `streamsFailed++` + dispError, then apply `failure_mode_allow` per
// parent §5.P11. The error is returned VERBATIM (wrapped with the stable
// prefix) — no swallowing, no silent recovery.
//
// **Concurrent-safe.** The package-level `unmarshalOpts` is read-only;
// protojson.Unmarshal is goroutine-safe. The caller may invoke
// unmarshalProcessingResponse concurrently from any goroutine.
//
// **Error wrapping.** Wraps the underlying protojson error with the
// stable prefix `"extproc: unmarshal ProcessingResponse: "` for
// grep-discoverability. The dispatcher at Task 8 further wraps with
// stage context for the per-stage error log.
func unmarshalProcessingResponse(data []byte) (*extprocsvcv3.ProcessingResponse, error) {
	resp := &extprocsvcv3.ProcessingResponse{}
	if err := unmarshalOpts.Unmarshal(data, resp); err != nil {
		return nil, fmt.Errorf("extproc: unmarshal ProcessingResponse: %w", err)
	}
	return resp, nil
}
