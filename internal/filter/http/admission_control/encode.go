package admission_control

// encode.go — StreamEncoderFilter methods for the admission_control filter per
// phase-23 SPEC §6.5 + §4.4 + AMEND-10 + AMEND-11 + ADR-0196.
//
// Replaces the pass-through stubs that Task 5 placed in admission_control.go.
// The compile-time assertion `var _ envoyhttp.StreamEncoderFilter = (*filter)(nil)`
// in admission_control.go still passes because these real methods are on the
// same *filter type.
//
// # Encode-path classification discipline (§4.4 + AMEND-10 + AMEND-11)
//
// EncodeHeaders entry points:
//  1. Guard on f.record (AMEND-11): if false → no-op + Continue.
//  2. Detect gRPC via Content-Type prefix "application/grpc" (PD-5).
//  3. gRPC path:
//     a. If grpc-status header is present (headers-encoded status, or endStream):
//        classify via cc.isGRPCSuccess(parsedStatus) → controller.classify(success).
//        f.expectGRPCStatusInTrailer stays false.
//     b. If grpc-status header is absent (status deferred to trailers):
//        set f.expectGRPCStatusInTrailer = true. Do NOT classify yet.
//  4. HTTP path (not gRPC): read the response status via f.ecb.ResponseStatus()
//     (ADR-0196 framework accessor — the encode header map does NOT carry
//     :status). Classify via cc.isHTTPSuccess(code). Unset/zero status → failure
//     (safe default). f.expectGRPCStatusInTrailer stays false.
//  5. Return Continue always.
//
// EncodeTrailers entry point:
//  1. Guard on f.record (AMEND-11): if false → no-op + TrailersContinue.
//  2. If f.expectGRPCStatusInTrailer: parse grpc-status from trailers,
//     classify via cc.isGRPCSuccess, call controller.classify(success).
//     Clear f.expectGRPCStatusInTrailer (prevents any double-classify if
//     EncodeTrailers fires again — though the framework does not, clearing
//     is tidy).
//  3. Return TrailersContinue.
//
// EncodeData: pass-through DataContinue.
// SetEncoderCallbacks: stores f.ecb (the HTTP path reads f.ecb.ResponseStatus()
//   per ADR-0196 — the encode header map does not carry :status).
// OnDestroy: no-op (admission_control has no token to release; no per-stream
//   cleanup needed).
//
// # Double-classify guard (AMEND-11 + LOCKED contract)
//
// The deferred-trailers path is exclusively gated by f.expectGRPCStatusInTrailer.
// The flag is only ever set when EncodeHeaders did NOT classify (no grpc-status
// header present). If EncodeHeaders classified (grpc-status in headers), the flag
// stays false, so EncodeTrailers is a no-op → no double-classify.
//
// # HTTP status + grpc-status access (ADR-0196 supersedes PD-5)
//
// HTTP status is read via f.ecb.ResponseStatus() (an int) — the ADR-0196
// encode-side framework accessor. PD-5's assumption that :status is available
// via headers.Get(":status") was INVALID: envoy-go's encode chain does not
// convey resp.Status to encode-side filters (the status lives in resp.Status
// at HCM dispatch and is written to the wire separately; compressor.go reads
// :status only as a best-effort optimization bucket, no-op when absent).
// gRPC status is read via headers.Get("Grpc-Status") / trailers.Get("Grpc-Status")
// and parsed with strconv.ParseUint — these are REAL headers/trailers that ARE
// present on the encode side (gRPC status codes are uint32 per proto3).
//
// # Cross-references
//
//   - SPEC §6.5 (EncodeHeaders / EncodeTrailers body)
//   - SPEC §4.4 (encode-path classification lemmata)
//   - AMEND-5 (HTTP default <500 / gRPC default 11-code set)
//   - AMEND-10 (gRPC-status in headers vs trailers per encodeHeaders/encodeTrailers)
//   - AMEND-11 (record discipline — f.record guards every classify call)
//   - PD-5 (encode-side status access — INVALID assumption; superseded by ADR-0196)
//   - ADR-0196 (ResponseStatus() encode-side framework accessor; the real status source)
//   - gRPC detection via Content-Type + Grpc-Status header/trailer (real headers)
//   - compiled_config.go (isHTTPSuccess / isGRPCSuccess predicates)
//   - controller.go (classify method — records into window + increments stats)
//   - admission_control.go (filter struct; expectGRPCStatusInTrailer field; record field)

import (
	"net/http"
	"strconv"
	"strings"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// SetEncoderCallbacks stores the encoder-side callbacks on the filter. The
// encode-path HTTP classification (EncodeHeaders below) reads the HTTP response
// status via f.ecb.ResponseStatus() (ADR-0196) — the encode header map handed
// to EncodeHeaders does NOT carry :status (it is resp.Status at HCM dispatch,
// written to the wire separately), so the framework accessor is the only source
// of the status code. The method MUST exist to satisfy the StreamEncoderFilter
// interface assertion in admission_control.go.
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }

// EncodeHeaders is the encode-side classification entry per SPEC §6.5 + §4.4
// + AMEND-10 + AMEND-11 + ADR-0196.
//
// Guards on f.record (AMEND-11): if false (rejected/disabled/health-check
// path set at DecodeHeaders), returns Continue without classifying.
//
// Otherwise detects gRPC via Content-Type prefix "application/grpc" and
// dispatches to HTTP or gRPC classification:
//   - gRPC + grpc-status present in headers: classify immediately.
//   - gRPC + no grpc-status header (deferred): set f.expectGRPCStatusInTrailer.
//   - HTTP (non-gRPC): read f.ecb.ResponseStatus() (ADR-0196) + classify.
func (f *filter) EncodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	// AMEND-11: guard — do NOT classify rejected/disabled/health-check requests.
	if !f.record {
		return envoyhttp.Continue
	}

	// PD-5 gRPC detection: Content-Type prefix "application/grpc".
	ct := headers.Get("Content-Type")
	if strings.HasPrefix(ct, "application/grpc") {
		// gRPC path: check for grpc-status header.
		grpcStatusStr := headers.Get("Grpc-Status")
		if grpcStatusStr != "" {
			// grpc-status present in headers (trailers-only / headers-encoded
			// response, or endStream) — classify immediately per AMEND-10.
			code, err := strconv.ParseUint(grpcStatusStr, 10, 32)
			success := err == nil && f.cc.isGRPCSuccess(uint32(code))
			f.controller.classify(success)
			// f.expectGRPCStatusInTrailer stays false (already classified).
		} else {
			// grpc-status absent — deferred to trailers per AMEND-10.
			// Set flag; EncodeTrailers will classify when the trailer arrives.
			f.expectGRPCStatusInTrailer = true
		}
		return envoyhttp.Continue
	}

	// HTTP path (non-gRPC): read the response status via the framework
	// accessor f.ecb.ResponseStatus() (ADR-0196).
	// HCM dispatch seeds the status via SetEncodeResponseStatus(resp.Status) per ADR-0196.
	var success bool
	if f.ecb != nil {
		code := f.ecb.ResponseStatus()
		if code > 0 {
			success = f.cc.isHTTPSuccess(code)
		}
		// code == 0 (unset status) → success stays false (failure; safe default,
		// preserving the prior missing-status semantics).
	}
	// f.ecb == nil (should not happen in production — the chain always calls
	// SetEncoderCallbacks before EncodeHeaders) → success stays false (failure;
	// safe default).
	f.controller.classify(success)
	return envoyhttp.Continue
}

// EncodeData is pass-through. admission_control has no body-time classification
// logic; the encode path classifies at EncodeHeaders (or EncodeTrailers for the
// gRPC-deferred path). Returns DataContinue.
func (f *filter) EncodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// EncodeTrailers handles the deferred gRPC-status classification case per
// AMEND-10 + AMEND-11 + SPEC §6.5.
//
// Guards on f.record (AMEND-11): if false → no-op + TrailersContinue.
//
// If f.expectGRPCStatusInTrailer is set (set at EncodeHeaders when a gRPC
// response had no grpc-status header), reads grpc-status from trailers,
// classifies via cc.isGRPCSuccess + controller.classify, and clears the flag.
// Otherwise (non-gRPC path, or already classified at EncodeHeaders) → no-op.
func (f *filter) EncodeTrailers(trailers http.Header) envoyhttp.FilterTrailersStatus {
	// AMEND-11: guard — do NOT classify rejected/disabled/health-check requests.
	if !f.record {
		return envoyhttp.TrailersContinue
	}

	// Double-classify guard (AMEND-10 + LOCKED contract): only classify at
	// trailers if the flag was set at EncodeHeaders (i.e., gRPC response with
	// no grpc-status header). If EncodeHeaders already classified, flag is false
	// and this is a no-op.
	if !f.expectGRPCStatusInTrailer {
		return envoyhttp.TrailersContinue
	}

	// Deferred gRPC classification: read grpc-status from trailers.
	grpcStatusStr := trailers.Get("Grpc-Status")
	var success bool
	if grpcStatusStr != "" {
		code, err := strconv.ParseUint(grpcStatusStr, 10, 32)
		success = err == nil && f.cc.isGRPCSuccess(uint32(code))
	}
	// grpc-status absent in trailers → classify as failure (safe default).
	f.controller.classify(success)

	// Clear the flag — classification is done; guard against any future spurious
	// EncodeTrailers calls (belt-and-suspenders; the framework calls it once).
	f.expectGRPCStatusInTrailer = false

	return envoyhttp.TrailersContinue
}

// OnDestroy is the per-stream destruction hook. admission_control has no
// token to release (unlike adaptive_concurrency's in-flight slot) and no
// per-stream cleanup state — this is a deliberate no-op at phase-23 MVP.
// See admission_control.go for the LOCKED contract documentation.
func (f *filter) OnDestroy() {}
