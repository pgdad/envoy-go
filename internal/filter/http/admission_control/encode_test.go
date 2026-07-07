package admission_control

// encode_test.go — Task 6 encode-side classification + record-discipline tests
// per phase-23 SPEC §6.5 + §4.4 + AMEND-10 + AMEND-11 + PD-5 + §14.1 items
// #5, #6, #7.
//
// # Test families
//
//  1. TestClassification_HTTP_* — EncodeHeaders HTTP path (status via the
//     f.ecb.ResponseStatus() accessor per ADR-0196 — NOT a :status header;
//     default <500 success + configured ranges); per SPEC §4.4 + AMEND-5.
//  2. TestClassification_GRPC_Headers_* — gRPC-status in response headers
//     (content-type: application/grpc + grpc-status header present); default
//     11-code set per AMEND-5 + AMEND-10.
//  3. TestClassification_GRPC_Trailers_* — gRPC-status deferred to trailers
//     (content-type: application/grpc, no grpc-status header at EncodeHeaders);
//     expectGRPCStatusInTrailer set + classified at EncodeTrailers per AMEND-10.
//  4. TestRecordDiscipline_NotRecordedWhenRejected — f.record=false ⇒ no
//     classify at EncodeHeaders; per AMEND-11.
//  5. TestRejectLocalReply_ByteShape — decode-path 503 + empty body + nil
//     headers per AMEND-7 + PD-2.503; named canonical §14.1 #7 test. Note:
//     overlaps TestDecodeHeaders_Reject_SendLocalReply_503_EmptyBody in
//     admission_control_test.go (Task 5); this test is the §14.1 #7 canonically-
//     named assertion.
//
// # Fakes consumed (from test-scope files per Task 3 — NOT redefined here)
//
//   - fakeRand (rand_test.go): fakeRand{v: uint64}
//   - clock.FakeClock (internal/clock): clock.NewFakeClock(start) + Advance(d)
//
// # testCompiledConfigAC / testFilterStatsAC consumed from controller_test.go.
//
// # acCallbacks / newACTestFilter consumed from admission_control_test.go.

import (
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/pgdad/envoy-go/internal/clock"
	"github.com/pgdad/envoy-go/internal/dynamicmetadata"
	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
)

// encodeTestCB is a test-double EncoderFilterCallbacks. The encode-path HTTP
// classification (encode.go) reads the response status via ResponseStatus()
// (ADR-0196 — the encode header map does NOT carry :status), so the HTTP
// classification tests drive the status via the settable status field here.
// All other methods are inert (the encode side of admission_control only
// consumes ResponseStatus()).
type encodeTestCB struct {
	status int
}

func (e *encodeTestCB) ContinueEncoding()                                  {}
func (e *encodeTestCB) EncodeHeaders(http.Header, bool)                    {}
func (e *encodeTestCB) EncodeData([]byte, bool)                            {}
func (e *encodeTestCB) EncodeTrailers(http.Header)                         {}
func (e *encodeTestCB) OverwriteBody([]byte)                               {}
func (e *encodeTestCB) BufferEncodedBody() []byte                          { return nil }
func (e *encodeTestCB) DownstreamRemoteAddr() net.Addr                     { return nil }
func (e *encodeTestCB) DownstreamLocalAddr() net.Addr                      { return nil }
func (e *encodeTestCB) DownstreamTLSServerName() string                    { return "" }
func (e *encodeTestCB) DownstreamTLSPeerCertDER() []byte                   { return nil }
func (e *encodeTestCB) DownstreamProtocol() string                         { return "" }
func (e *encodeTestCB) ListenerPrincipal() string                          { return "" }
func (e *encodeTestCB) DownstreamTLSConnectionState() *tls.ConnectionState { return nil }
func (e *encodeTestCB) DynamicMetadata() *dynamicmetadata.Bucket           { return nil }
func (e *encodeTestCB) ResponseStatus() int                                { return e.status }

// newEncodeTestFilter constructs a *filter wired for encode-path tests.
// The filter's record is set per the recordEnabled argument. controller uses
// the provided Rand. The filter is wired with an *encodeTestCB so the HTTP
// classification path can read the response status via f.ecb.ResponseStatus()
// (ADR-0196). Tests set the HTTP status via setEncodeHTTPStatus before calling
// EncodeHeaders on the HTTP (non-gRPC) path.
func newEncodeTestFilter(t *testing.T, rnd Rand, recordEnabled bool) (*filter, *controller) {
	t.Helper()
	cfg := testCompiledConfigAC()
	clock := clock.NewFakeClock(time.Unix(0, 0))
	st := newFilterStats(stats.NewRegistry(), "http.test")
	ctrl := newController(cfg, st, clock, rnd)
	f := &filter{
		cc:         cfg,
		controller: ctrl,
		stats:      st,
		record:     recordEnabled,
		ecb:        &encodeTestCB{},
	}
	return f, ctrl
}

// setEncodeHTTPStatus sets the HTTP response status the encode-side accessor
// (f.ecb.ResponseStatus()) will return for the HTTP classification path. Mirrors
// HCM dispatch's chain.SetEncodeResponseStatus(resp.Status) seeding (ADR-0196).
func setEncodeHTTPStatus(t *testing.T, f *filter, code int) {
	t.Helper()
	cb, ok := f.ecb.(*encodeTestCB)
	if !ok {
		t.Fatalf("setEncodeHTTPStatus: f.ecb is %T, want *encodeTestCB", f.ecb)
	}
	cb.status = code
}

// httpHeaders builds a minimal http.Header with the given key-value pairs.
func httpHeaders(pairs ...string) http.Header {
	h := make(http.Header)
	for i := 0; i+1 < len(pairs); i += 2 {
		h.Set(pairs[i], pairs[i+1])
	}
	return h
}

// ============================================================================
// 1. TestClassification_HTTP_* — EncodeHeaders HTTP status classification
// ============================================================================

// TestClassification_HTTP_DefaultSuccess_LT500 verifies that HTTP status codes
// in the range [100,500) are classified as success (increment rqSuccess) per
// AMEND-5's default success criterion.
func TestClassification_HTTP_DefaultSuccess_LT500(t *testing.T) {
	successCodes := []int{100, 200, 201, 301, 404, 499}
	for _, code := range successCodes {
		t.Run("status_"+strconv.Itoa(code), func(t *testing.T) {
			f, _ := newEncodeTestFilter(t, neverRejectRand(), true)

			before := f.stats.rqSuccess.Load()
			// Status is supplied via the encode-side accessor (ADR-0196), not a
			// :status header. The encode header map is HTTP (non-gRPC) — empty.
			setEncodeHTTPStatus(t, f, code)
			headers := http.Header{}

			status := f.EncodeHeaders(headers, false)

			if status != envoyhttp.Continue {
				t.Errorf("EncodeHeaders status: got %v, want Continue", status)
			}
			if got := f.stats.rqSuccess.Load(); got != before+1 {
				t.Errorf("rqSuccess: got %d, want %d (success classification for HTTP %d)", got, before+1, code)
			}
			if got := f.stats.rqFailure.Load(); got != 0 {
				t.Errorf("rqFailure: got %d, want 0 (must NOT increment on success)", got)
			}
		})
	}
}

// TestClassification_HTTP_DefaultFailure_GTE500 verifies that HTTP status codes
// >= 500 are classified as failure (increment rqFailure) per AMEND-5's default.
func TestClassification_HTTP_DefaultFailure_GTE500(t *testing.T) {
	failureCodes := []int{500, 502, 503, 504, 599}
	for _, code := range failureCodes {
		t.Run("status_"+formatHTTPStatus(code), func(t *testing.T) {
			f, _ := newEncodeTestFilter(t, neverRejectRand(), true)

			before := f.stats.rqFailure.Load()
			// Status via the encode-side accessor (ADR-0196), not a :status header.
			setEncodeHTTPStatus(t, f, code)
			headers := http.Header{}

			status := f.EncodeHeaders(headers, false)

			if status != envoyhttp.Continue {
				t.Errorf("EncodeHeaders status: got %v, want Continue", status)
			}
			if got := f.stats.rqFailure.Load(); got != before+1 {
				t.Errorf("rqFailure: got %d, want %d (failure classification for HTTP %d)", got, before+1, code)
			}
			if got := f.stats.rqSuccess.Load(); got != 0 {
				t.Errorf("rqSuccess: got %d, want 0 (must NOT increment on failure)", got)
			}
		})
	}
}

// TestClassification_HTTP_ConfiguredRange verifies that a configured HTTP
// success range [100, 600) classifies all codes 100..599 as success.
func TestClassification_HTTP_ConfiguredRange(t *testing.T) {
	f, ctrl := newEncodeTestFilter(t, neverRejectRand(), true)
	// Reconfigure: success range = [200, 300)
	f.cc.httpSuccessRanges = []int32Range{{start: 200, end: 300}}

	setEncodeHTTPStatus(t, f, 200)
	_ = f.EncodeHeaders(http.Header{}, false)
	if got := f.stats.rqSuccess.Load(); got != 1 {
		t.Errorf("rqSuccess after 200 in [200,300): got %d, want 1", got)
	}
	// M4: window must record the request (n=1, s=1 for the success).
	if n, s := ctrl.requestCounts(); n != 1 || s != 1 {
		t.Errorf("window after HTTP success: requestCounts()=(%d,%d); want (1,1)", n, s)
	}

	setEncodeHTTPStatus(t, f, 500)
	_ = f.EncodeHeaders(http.Header{}, false)
	if got := f.stats.rqFailure.Load(); got != 1 {
		t.Errorf("rqFailure after 500 outside [200,300): got %d, want 1", got)
	}
}

// TestClassification_HTTP_NilCallbacks verifies the defensive nil-ecb guard
// (ADR-0196): if SetEncoderCallbacks was never called (should not happen in
// production — the chain always wires the callbacks before EncodeHeaders), the
// HTTP path treats the request as failure (safe default, preserving the prior
// missing-status semantics).
func TestClassification_HTTP_NilCallbacks(t *testing.T) {
	f, _ := newEncodeTestFilter(t, neverRejectRand(), true)
	f.ecb = nil // simulate the should-not-happen un-wired path

	status := f.EncodeHeaders(http.Header{}, false)

	if status != envoyhttp.Continue {
		t.Errorf("EncodeHeaders status: got %v, want Continue", status)
	}
	// nil ecb → classify as failure (safe default)
	if got := f.stats.rqFailure.Load(); got != 1 {
		t.Errorf("rqFailure after nil ecb: got %d, want 1", got)
	}
}

// TestClassification_HTTP_ZeroStatus verifies that an unset (zero) response
// status — what ResponseStatus() returns for a synthetic stream that HCM
// dispatch never seeded (ADR-0196 zero-value semantics) — is treated as failure
// (safe default, preserving the prior missing-:status semantics).
func TestClassification_HTTP_ZeroStatus(t *testing.T) {
	f, _ := newEncodeTestFilter(t, neverRejectRand(), true)
	setEncodeHTTPStatus(t, f, 0) // unset status

	status := f.EncodeHeaders(http.Header{}, false)

	if status != envoyhttp.Continue {
		t.Errorf("EncodeHeaders status: got %v, want Continue", status)
	}
	if got := f.stats.rqFailure.Load(); got != 1 {
		t.Errorf("rqFailure after zero status: got %d, want 1", got)
	}
}

// formatHTTPStatus converts an integer HTTP code to its string representation.
func formatHTTPStatus(code int) string { return strconv.Itoa(code) }

// ============================================================================
// 2. TestClassification_GRPC_Headers_* — gRPC-status in response headers
// ============================================================================

// TestClassification_GRPC_Headers_DefaultSuccessCodes verifies that the 11
// default gRPC success codes {0,1,2,3,5,6,7,9,11,12,16} are classified as
// success when grpc-status is present in response headers per AMEND-5 + AMEND-10.
func TestClassification_GRPC_Headers_DefaultSuccessCodes(t *testing.T) {
	successCodes := []uint32{0, 1, 2, 3, 5, 6, 7, 9, 11, 12, 16}
	for _, code := range successCodes {
		t.Run(formatGRPCCode(code), func(t *testing.T) {
			f, _ := newEncodeTestFilter(t, neverRejectRand(), true)

			headers := grpcResponseHeaders(code)
			before := f.stats.rqSuccess.Load()

			status := f.EncodeHeaders(headers, false)

			if status != envoyhttp.Continue {
				t.Errorf("EncodeHeaders status: got %v, want Continue", status)
			}
			if got := f.stats.rqSuccess.Load(); got != before+1 {
				t.Errorf("rqSuccess: got %d, want %d (gRPC code %d in default 11-code set)", got, before+1, code)
			}
			if got := f.stats.rqFailure.Load(); got != 0 {
				t.Errorf("rqFailure: got %d, want 0 (must NOT increment on gRPC success)", got)
			}
		})
	}
}

// TestClassification_GRPC_Headers_DefaultFailureCodes verifies that gRPC codes
// NOT in the default 11-code set are classified as failure per AMEND-5.
// Codes NOT in {0,1,2,3,5,6,7,9,11,12,16}: {4,8,10,13,14,15}.
func TestClassification_GRPC_Headers_DefaultFailureCodes(t *testing.T) {
	failureCodes := []uint32{4, 8, 10, 13, 14, 15}
	for _, code := range failureCodes {
		t.Run(formatGRPCCode(code), func(t *testing.T) {
			f, _ := newEncodeTestFilter(t, neverRejectRand(), true)

			headers := grpcResponseHeaders(code)
			before := f.stats.rqFailure.Load()

			status := f.EncodeHeaders(headers, false)

			if status != envoyhttp.Continue {
				t.Errorf("EncodeHeaders status: got %v, want Continue", status)
			}
			if got := f.stats.rqFailure.Load(); got != before+1 {
				t.Errorf("rqFailure: got %d, want %d (gRPC code %d NOT in default 11-code set)", got, before+1, code)
			}
			if got := f.stats.rqSuccess.Load(); got != 0 {
				t.Errorf("rqSuccess: got %d, want 0 (must NOT increment on gRPC failure)", got)
			}
		})
	}
}

// TestClassification_GRPC_Headers_EndStream verifies that a gRPC response with
// grpc-status in headers + endStream=true is classified immediately at
// EncodeHeaders (trailers-only / headers-encoded response path per AMEND-10).
func TestClassification_GRPC_Headers_EndStream(t *testing.T) {
	f, ctrl := newEncodeTestFilter(t, neverRejectRand(), true)

	// gRPC OK (code=0) with endStream=true: trailers-only response.
	headers := grpcResponseHeaders(0)

	status := f.EncodeHeaders(headers, true /* endStream */)

	if status != envoyhttp.Continue {
		t.Errorf("EncodeHeaders status: got %v, want Continue", status)
	}
	if got := f.stats.rqSuccess.Load(); got != 1 {
		t.Errorf("rqSuccess: got %d, want 1 (gRPC OK endStream classified at EncodeHeaders)", got)
	}
	// expectGRPCStatusInTrailer must NOT be set (already classified at headers).
	if f.expectGRPCStatusInTrailer {
		t.Error("expectGRPCStatusInTrailer = true after gRPC classify at EncodeHeaders; want false")
	}
	// M4: window must record the request (n=1, s=1 for the gRPC success).
	if n, s := ctrl.requestCounts(); n != 1 || s != 1 {
		t.Errorf("window after gRPC-headers success: requestCounts()=(%d,%d); want (1,1)", n, s)
	}
}

// grpcResponseHeaders builds a minimal http.Header for a gRPC response with
// the given status code in the grpc-status header.
func grpcResponseHeaders(grpcCode uint32) http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/grpc")
	h.Set("Grpc-Status", formatGRPCCode(grpcCode))
	return h
}

// formatGRPCCode converts a gRPC status code to its string representation.
func formatGRPCCode(code uint32) string { return strconv.FormatUint(uint64(code), 10) }

// ============================================================================
// 3. TestClassification_GRPC_Trailers_* — gRPC-status deferred to trailers
// ============================================================================

// TestClassification_GRPC_Trailers_Success verifies the AMEND-10 deferred-
// trailers path: EncodeHeaders detects gRPC content-type but no grpc-status
// header ⇒ sets expectGRPCStatusInTrailer; EncodeTrailers reads grpc-status
// and classifies success.
func TestClassification_GRPC_Trailers_Success(t *testing.T) {
	f, ctrl := newEncodeTestFilter(t, neverRejectRand(), true)

	// EncodeHeaders: gRPC response without grpc-status header ⇒ defer.
	headers := http.Header{}
	headers.Set("Content-Type", "application/grpc")
	// No Grpc-Status header

	status := f.EncodeHeaders(headers, false)

	if status != envoyhttp.Continue {
		t.Errorf("EncodeHeaders status: got %v, want Continue", status)
	}
	// No classification yet at EncodeHeaders.
	if got := f.stats.rqSuccess.Load(); got != 0 {
		t.Errorf("rqSuccess: got %d, want 0 (no classification before trailers)", got)
	}
	if got := f.stats.rqFailure.Load(); got != 0 {
		t.Errorf("rqFailure: got %d, want 0 (no classification before trailers)", got)
	}
	// expectGRPCStatusInTrailer must be set.
	if !f.expectGRPCStatusInTrailer {
		t.Error("expectGRPCStatusInTrailer = false after gRPC headers without grpc-status; want true")
	}

	// EncodeTrailers: grpc-status = 0 (OK) ⇒ success classification.
	trailers := http.Header{}
	trailers.Set("Grpc-Status", "0")

	trailerStatus := f.EncodeTrailers(trailers)

	if trailerStatus != envoyhttp.TrailersContinue {
		t.Errorf("EncodeTrailers status: got %v, want TrailersContinue", trailerStatus)
	}
	if got := f.stats.rqSuccess.Load(); got != 1 {
		t.Errorf("rqSuccess: got %d, want 1 (gRPC OK in trailers classified at EncodeTrailers)", got)
	}
	if got := f.stats.rqFailure.Load(); got != 0 {
		t.Errorf("rqFailure: got %d, want 0 (must NOT increment on gRPC success in trailers)", got)
	}
	// M4: window must record the request (n=1, s=1 for the gRPC trailers success).
	if n, s := ctrl.requestCounts(); n != 1 || s != 1 {
		t.Errorf("window after gRPC-trailers success: requestCounts()=(%d,%d); want (1,1)", n, s)
	}
}

// TestClassification_GRPC_Trailers_Failure verifies the AMEND-10 deferred-
// trailers path for a failure code: grpc-status = 4 (DeadlineExceeded) is NOT
// in the default 11-code success set ⇒ classified as failure at EncodeTrailers.
func TestClassification_GRPC_Trailers_Failure(t *testing.T) {
	f, _ := newEncodeTestFilter(t, neverRejectRand(), true)

	// EncodeHeaders: gRPC response without grpc-status header ⇒ defer.
	headers := http.Header{}
	headers.Set("Content-Type", "application/grpc+proto")
	// No Grpc-Status header

	_ = f.EncodeHeaders(headers, false)

	if !f.expectGRPCStatusInTrailer {
		t.Fatal("expectGRPCStatusInTrailer = false after gRPC headers without grpc-status; want true")
	}

	// EncodeTrailers: grpc-status = 4 (DeadlineExceeded) ⇒ failure.
	trailers := http.Header{}
	trailers.Set("Grpc-Status", "4")

	_ = f.EncodeTrailers(trailers)

	if got := f.stats.rqFailure.Load(); got != 1 {
		t.Errorf("rqFailure: got %d, want 1 (gRPC code 4 NOT in default 11-code set)", got)
	}
	if got := f.stats.rqSuccess.Load(); got != 0 {
		t.Errorf("rqSuccess: got %d, want 0 (must NOT increment on gRPC failure in trailers)", got)
	}
}

// TestClassification_GRPC_Trailers_NoDoubleClassify verifies the double-classify
// guard (AMEND-11 + LOCKED contract): if EncodeHeaders already classified (e.g.
// grpc-status was present in headers), EncodeTrailers must NOT classify again.
func TestClassification_GRPC_Trailers_NoDoubleClassify(t *testing.T) {
	f, _ := newEncodeTestFilter(t, neverRejectRand(), true)

	// EncodeHeaders: grpc-status=0 present in headers ⇒ classify immediately.
	headers := grpcResponseHeaders(0) // grpc-status: 0
	_ = f.EncodeHeaders(headers, false)

	if got := f.stats.rqSuccess.Load(); got != 1 {
		t.Fatalf("rqSuccess after EncodeHeaders with grpc-status=0: got %d, want 1", got)
	}
	// expectGRPCStatusInTrailer must NOT be set (already classified at headers).
	if f.expectGRPCStatusInTrailer {
		t.Fatal("expectGRPCStatusInTrailer = true after classify at EncodeHeaders; want false")
	}

	// EncodeTrailers fires (e.g., framework always calls it) — must be no-op.
	trailers := http.Header{}
	trailers.Set("Grpc-Status", "4") // a failure code — must NOT re-classify

	_ = f.EncodeTrailers(trailers)

	// rqSuccess must still be 1 (no re-increment); rqFailure must be 0 (no new classify).
	if got := f.stats.rqSuccess.Load(); got != 1 {
		t.Errorf("rqSuccess after EncodeTrailers (no-op): got %d, want 1 (no double-classify)", got)
	}
	if got := f.stats.rqFailure.Load(); got != 0 {
		t.Errorf("rqFailure after EncodeTrailers (no-op): got %d, want 0 (no double-classify)", got)
	}
}

// TestClassification_GRPC_Trailers_NonGRPC_EncodeTrailers verifies that
// EncodeTrailers is a pass-through no-op for non-gRPC requests
// (expectGRPCStatusInTrailer = false from the start).
func TestClassification_GRPC_Trailers_NonGRPC_EncodeTrailers(t *testing.T) {
	f, _ := newEncodeTestFilter(t, neverRejectRand(), true)
	// HTTP (non-gRPC) response: expectGRPCStatusInTrailer = false.
	setEncodeHTTPStatus(t, f, 200)
	_ = f.EncodeHeaders(http.Header{}, false)

	if f.expectGRPCStatusInTrailer {
		t.Fatal("expectGRPCStatusInTrailer = true after HTTP EncodeHeaders; want false")
	}
	// Clear counters for EncodeTrailers check.
	// rqSuccess should be 1 from EncodeHeaders; rqFailure = 0.
	beforeSuccess := f.stats.rqSuccess.Load()

	trailers := http.Header{}
	trailers.Set("Grpc-Status", "4") // irrelevant for non-gRPC
	_ = f.EncodeTrailers(trailers)

	if got := f.stats.rqSuccess.Load(); got != beforeSuccess {
		t.Errorf("rqSuccess changed in EncodeTrailers no-op: got %d, want %d", got, beforeSuccess)
	}
	if got := f.stats.rqFailure.Load(); got != 0 {
		t.Errorf("rqFailure changed in EncodeTrailers no-op: got %d, want 0", got)
	}
}

// ============================================================================
// 4. TestRecordDiscipline_NotRecordedWhenRejected — f.record=false ⇒ no-op
// ============================================================================

// TestRecordDiscipline_NotRecordedWhenRejected verifies that when f.record=false
// (rejected / disabled / health-check path per AMEND-11), EncodeHeaders does NOT
// classify: no rqSuccess/rqFailure increment + no window recordRequest.
func TestRecordDiscipline_NotRecordedWhenRejected(t *testing.T) {
	f, ctrl := newEncodeTestFilter(t, neverRejectRand(), false /* record=false */)

	// HTTP 200 headers — would be a success if f.record were true.
	headers := httpHeaders(":status", "200")
	status := f.EncodeHeaders(headers, false)

	if status != envoyhttp.Continue {
		t.Errorf("EncodeHeaders status: got %v, want Continue (pass-through when record=false)", status)
	}
	if got := f.stats.rqSuccess.Load(); got != 0 {
		t.Errorf("rqSuccess: got %d, want 0 (record=false ⇒ no classify per AMEND-11)", got)
	}
	if got := f.stats.rqFailure.Load(); got != 0 {
		t.Errorf("rqFailure: got %d, want 0 (record=false ⇒ no classify per AMEND-11)", got)
	}

	// Verify no window state mutated.
	n, s := ctrl.requestCounts()
	if n != 0 || s != 0 {
		t.Errorf("window state mutated on record=false: requests=%d successes=%d; want 0,0", n, s)
	}
}

// TestRecordDiscipline_NotRecordedWhenRejected_GRPC verifies the same for a
// gRPC response with f.record=false.
func TestRecordDiscipline_NotRecordedWhenRejected_GRPC(t *testing.T) {
	f, ctrl := newEncodeTestFilter(t, neverRejectRand(), false /* record=false */)

	// gRPC OK headers — would be success if f.record were true.
	headers := grpcResponseHeaders(0)
	_ = f.EncodeHeaders(headers, false)

	if got := f.stats.rqSuccess.Load(); got != 0 {
		t.Errorf("rqSuccess: got %d, want 0 (record=false ⇒ no classify per AMEND-11)", got)
	}

	// EncodeTrailers should also be a no-op even if expectGRPCStatusInTrailer
	// were set (but it shouldn't be, since we short-circuited at record=false).
	trailers := http.Header{}
	trailers.Set("Grpc-Status", "0")
	_ = f.EncodeTrailers(trailers)

	n, s := ctrl.requestCounts()
	if n != 0 || s != 0 {
		t.Errorf("window state mutated on record=false GRPC: requests=%d successes=%d; want 0,0", n, s)
	}
}

// TestRecordDiscipline_NotRecordedWhenRejected_Trailers verifies that
// EncodeTrailers is also a no-op when f.record=false (even if
// expectGRPCStatusInTrailer had somehow been set before record was cleared).
func TestRecordDiscipline_NotRecordedWhenRejected_Trailers(t *testing.T) {
	f, _ := newEncodeTestFilter(t, neverRejectRand(), false /* record=false */)

	// Manually set expectGRPCStatusInTrailer to simulate a race condition or
	// unexpected path — encode side must still guard on f.record.
	f.expectGRPCStatusInTrailer = true

	trailers := http.Header{}
	trailers.Set("Grpc-Status", "0")
	_ = f.EncodeTrailers(trailers)

	if got := f.stats.rqSuccess.Load(); got != 0 {
		t.Errorf("rqSuccess: got %d, want 0 (record=false guards EncodeTrailers per AMEND-11)", got)
	}
	if got := f.stats.rqFailure.Load(); got != 0 {
		t.Errorf("rqFailure: got %d, want 0 (record=false guards EncodeTrailers per AMEND-11)", got)
	}
}

// ============================================================================
// 5. TestRejectLocalReply_ByteShape — decode-path 503 byte-shape assertion
// ============================================================================

// TestRejectLocalReply_ByteShape is the §14.1 #7 canonically-named test
// asserting the decode-path reject byte shape per AMEND-7 + PD-2.503:
// status 503, empty body, nil headers.
//
// Note: this overlaps TestDecodeHeaders_Reject_SendLocalReply_503_EmptyBody in
// admission_control_test.go (Task 5 landing). Both tests assert the same SPEC
// §14.1 #7 requirement; this one is the §14.1-spec-referenced canonical name.
// The overlap is intentional per the PLAN's "add the named test; assert same
// shape; note in PROGRESS if it overlaps Task 5's reject test" instruction.
func TestRejectLocalReply_ByteShape(t *testing.T) {
	f, ctrl, _, cb := newACTestFilter(t, alwaysRejectRand())
	f.cc.rpsThreshold = 0 // RPS gate passes (empty window ⇒ averageRps=0)
	primeWindowForRejection(t, ctrl)

	// Execute the reject gate via DecodeHeaders.
	decodeStatus := f.DecodeHeaders(http.Header{}, false)

	// Verify StopIteration — the reject path.
	if decodeStatus != envoyhttp.StopIteration {
		t.Errorf("DecodeHeaders status: got %v, want StopIteration (reject gate)", decodeStatus)
	}

	// Verify SendLocalReply was called.
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want invocation on reject; got nil")
	}

	// AMEND-7 + PD-2.503 byte-pinned reject wire shape.
	if cb.localReply.status != 503 {
		t.Errorf("SendLocalReply status: got %d, want 503 (AMEND-7 + PD-2.503)", cb.localReply.status)
	}
	if cb.localReply.body != "" {
		t.Errorf("SendLocalReply body: got %q, want %q (AMEND-7 empty body)", cb.localReply.body, "")
	}
	if cb.localReply.headers != nil {
		t.Errorf("SendLocalReply headers: got %v, want nil (PD-2.503 no added headers)", cb.localReply.headers)
	}

	// After reject: f.record must be false (AMEND-11 — rejected request not recorded).
	if f.record {
		t.Error("f.record = true after reject; want false (AMEND-11)")
	}

	// EncodeHeaders must be a no-op (f.record=false guards classify per AMEND-11).
	successBefore := f.stats.rqSuccess.Load()
	failureBefore := f.stats.rqFailure.Load()
	_ = f.EncodeHeaders(httpHeaders(":status", "200"), false)
	if got := f.stats.rqSuccess.Load(); got != successBefore {
		t.Errorf("rqSuccess changed after EncodeHeaders on rejected request: got %d, want %d (no-op per AMEND-11)", got, successBefore)
	}
	if got := f.stats.rqFailure.Load(); got != failureBefore {
		t.Errorf("rqFailure changed after EncodeHeaders on rejected request: got %d, want %d (no-op per AMEND-11)", got, failureBefore)
	}
}

// ============================================================================
// 6. EncodeData + SetEncoderCallbacks pass-through tests
// ============================================================================

// TestEncodeData_PassThrough verifies that EncodeData returns DataContinue
// without touching any counters.
func TestEncodeData_PassThrough(t *testing.T) {
	f, _ := newEncodeTestFilter(t, neverRejectRand(), true)

	status := f.EncodeData([]byte("body"), false)

	if status != envoyhttp.DataContinue {
		t.Errorf("EncodeData status: got %v, want DataContinue", status)
	}
	if got := f.stats.rqSuccess.Load(); got != 0 {
		t.Errorf("rqSuccess: got %d, want 0 (EncodeData must not classify)", got)
	}
}

// TestSetEncoderCallbacks_Stores verifies that SetEncoderCallbacks stores the
// supplied callbacks on f.ecb (ADR-0196 — the HTTP classification path reads
// f.ecb.ResponseStatus()) and that a subsequent HTTP EncodeHeaders observes the
// stored callbacks' status.
func TestSetEncoderCallbacks_Stores(t *testing.T) {
	f, _ := newEncodeTestFilter(t, neverRejectRand(), true)

	// SetEncoderCallbacks stores the supplied callbacks.
	cb := &encodeTestCB{status: 200}
	f.SetEncoderCallbacks(cb)
	if f.ecb != envoyhttp.EncoderFilterCallbacks(cb) {
		t.Fatalf("SetEncoderCallbacks did not store the callbacks: f.ecb=%v", f.ecb)
	}

	// HTTP EncodeHeaders reads the stored callbacks' status (200 → success).
	status := f.EncodeHeaders(http.Header{}, false)
	if status != envoyhttp.Continue {
		t.Errorf("EncodeHeaders after SetEncoderCallbacks: got %v, want Continue", status)
	}
	if got := f.stats.rqSuccess.Load(); got != 1 {
		t.Errorf("rqSuccess: got %d, want 1 (status 200 from stored callbacks)", got)
	}
}

// TestOnDestroy_NoOp verifies that OnDestroy is a no-op (admission_control has
// no token to release, no state to tear down per SPEC §6.5 + the LOCKED contract).
func TestOnDestroy_NoOp(t *testing.T) {
	f, _ := newEncodeTestFilter(t, neverRejectRand(), true)

	// OnDestroy should not panic and should not modify any counters.
	before := f.stats.rqSuccess.Load()
	f.OnDestroy()
	if got := f.stats.rqSuccess.Load(); got != before {
		t.Errorf("rqSuccess changed in OnDestroy: got %d, want %d", got, before)
	}
}
