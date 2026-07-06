package admission_control

// admission_control_test.go — TypeURL byte-exact pin + DecodeHeaders gate tests
// per phase-23 SPEC §6.4 + AMEND-7 + AMEND-11 + PD-2.503 + PD-3 + §14.1.
//
// # Test taxonomy
//
//  1. TestTypeURL_ByteExact — pins the TypeURL constant byte-exact per ADR-0143
//     SN1 (byte-exact regression guard). Mirrors adaptive_concurrency precedent.
//  2. TestDecodeHeaders_Disabled_PassThrough — cc.enabled=false ⇒ Continue +
//     record cleared (f.record=false); no rqRejected increment per AMEND-11.
//  3. TestDecodeHeaders_RpsSuppression — averageRps() < rpsThreshold ⇒ Continue
//     with f.record stays true (NOT a reject; request proceeds to encode).
//  4. TestDecodeHeaders_Reject_Increments_rqRejected — shouldReject via injected
//     fakeRand ⇒ StopIteration + rqRejected.Inc() + record cleared (f.record=false).
//  5. TestDecodeHeaders_Reject_SendLocalReply_503_EmptyBody — reject path:
//     SendLocalReply(503, "", nil) per AMEND-7 + PD-2.503.
//  6. TestDecodeHeaders_Admit_PassThrough — shouldReject=false via injected fakeRand
//     ⇒ Continue + f.record stays true.
//
// # NOT re-asserted (Task 3 OWNS stat names)
//
// Stat-name byte-exactness is asserted in stats_test.go (Task 3). This file does
// NOT re-assert those names.
//
// # Fakes consumed (from test-scope files per Task 3 — NOT redefined here)
//
//   - fakeRand (rand_test.go): fakeRand{v: uint64} — deterministic Rand seam
//   - fakeClock (clock_test.go): newFakeClock(start) + Advance(d)
//
// DO NOT redefine fakeRand or fakeClock here.

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"google.golang.org/protobuf/proto"

	"github.com/pgdad/envoy-go/internal/dynamicmetadata"
	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
)

// -----------------------------------------------------------------------------
// acLocalReplyArgs / acCallbacks — test-double DecoderFilterCallbacks for
// SendLocalReply capture. Mirrors adaptive_concurrency decode_headers_test.go
// recordedCallbacks pattern. All methods unused by admission_control at decode
// time return zero values.
// -----------------------------------------------------------------------------

// acLocalReplyArgs captures one SendLocalReply invocation.
type acLocalReplyArgs struct {
	status  int
	body    string
	headers envoyhttp.OrderedHeaders
}

// acCallbacks is a test-double DecoderFilterCallbacks that records SendLocalReply
// invocations. All callback methods not exercised by admission_control's decode
// path return zero values.
type acCallbacks struct {
	mu         sync.Mutex
	localReply *acLocalReplyArgs // non-nil if SendLocalReply was called
}

func (c *acCallbacks) ContinueDecoding() {}

func (c *acCallbacks) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.localReply = &acLocalReplyArgs{status: status, body: body, headers: headers}
}

func (c *acCallbacks) RequestRouteConfig() proto.Message { return nil }
func (c *acCallbacks) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (c *acCallbacks) EncodeHeaders(http.Header, bool) {}
func (c *acCallbacks) EncodeData([]byte, bool)         {}
func (c *acCallbacks) EncodeTrailers(http.Header)      {}
func (c *acCallbacks) DownstreamPrincipal() []string   { return nil }

// ADR-0165 callback-surface extension stubs.
func (c *acCallbacks) DownstreamRemoteAddr() net.Addr   { return nil }
func (c *acCallbacks) DownstreamLocalAddr() net.Addr    { return nil }
func (c *acCallbacks) DownstreamTLSServerName() string  { return "" }
func (c *acCallbacks) DownstreamTLSPeerCertDER() []byte { return nil }
func (c *acCallbacks) DownstreamProtocol() string       { return "" }
func (c *acCallbacks) ListenerPrincipal() string        { return "" }

// ADR-0192 callback-surface extension stubs.
func (c *acCallbacks) DownstreamTLSConnectionState() *tls.ConnectionState { return nil }
func (c *acCallbacks) DynamicMetadata() *dynamicmetadata.Bucket           { return nil }

// ADR-0198 callback-surface extension stubs (phase-24.1 Task 5 — DELTA-2).
func (c *acCallbacks) RouteRateLimits() []*routev3.RateLimit       { return nil }
func (c *acCallbacks) VirtualHostRateLimits() []*routev3.RateLimit { return nil }
func (c *acCallbacks) RouteMetadata() *corev3.Metadata             { return nil }
func (c *acCallbacks) RouteIncludeVhRateLimits() bool              { return false }

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

// newACTestFilter constructs a *filter + *controller wired with a fakeClock and
// the provided fakeRand. The filter's record is true at construction (default
// per AMEND-11 + PD-4: record=true out of the gate). The controller starts with
// an empty window.
//
// cfg.rpsThreshold defaults to 0 in testCompiledConfigAC() so the RPS gate never
// suppresses unless the test sets rpsThreshold explicitly.
func newACTestFilter(t *testing.T, rnd Rand) (*filter, *controller, *fakeClock, *acCallbacks) {
	t.Helper()
	cfg := testCompiledConfigAC()
	clock := newFakeClock(time.Unix(0, 0))
	st := newFilterStats(stats.NewRegistry(), "http.test")
	ctrl := newController(cfg, st, clock, rnd)
	cb := &acCallbacks{}
	f := &filter{
		cc:         cfg,
		controller: ctrl,
		stats:      st,
		record:     true,
	}
	f.SetDecoderCallbacks(cb)
	return f, ctrl, clock, cb
}

// neverRejectRand returns a fakeRand value that makes shouldReject() always
// return false: r%10000 == 9999, so float64(10000)*P > 9999 is only true when
// P > 0.9999 (above defaultMaxRejectionProbability=0.80). With default params
// and an empty window (P=0), shouldReject() returns false for any r.
func neverRejectRand() fakeRand { return fakeRand{v: 9999} }

// alwaysRejectRand returns a fakeRand value that makes shouldReject() always
// return true when P > 0: r%10000 == 0, so float64(10000)*P > 0 is true for
// any P > 0.
//
// To guarantee rejection via shouldReject, the window must be non-empty with
// some failures (so P > 0). We prime the window with enough failures to push
// P above 0 before calling DecodeHeaders.
func alwaysRejectRand() fakeRand { return fakeRand{v: 0} }

// primeWindowForRejection fills the controller's window with 10 failures
// (success=false) to ensure shouldReject() returns a P > 0 value. With
// srThreshold=0.95 and all failures: inner = (10 - 0/0.95)/(10+1) ≈ 0.909.
// With aggression=1.0, P=inner≈0.909 > 0. float64(10000)*0.909 > 0 → TRUE.
func primeWindowForRejection(t *testing.T, ctrl *controller) {
	t.Helper()
	for i := 0; i < 10; i++ {
		ctrl.recordRequest(false)
	}
}

// -----------------------------------------------------------------------------
// 1. TestTypeURL_ByteExact — ADR-0143 SN1 byte-exact pin.
// -----------------------------------------------------------------------------

// TestTypeURL_ByteExact pins the TypeURL constant byte-exact per ADR-0143 SN1.
// The wire-name is consumed by the HCM filter-chain builder to resolve typed_config
// Any envelopes against the HTTPRegistry's frozen factory map; any drift surfaces
// as a runtime "no factory registered for type URL" error rather than a build-time
// failure. This compile-time constant assertion catches drift at `go test` time.
func TestTypeURL_ByteExact(t *testing.T) {
	const want = "type.googleapis.com/envoy.extensions.filters.http.admission_control.v3.AdmissionControl"
	if TypeURL != want {
		t.Errorf("TypeURL drift: got %q; want %q", TypeURL, want)
	}
}

// -----------------------------------------------------------------------------
// 2. TestDecodeHeaders_Disabled_PassThrough
// -----------------------------------------------------------------------------

// TestDecodeHeaders_Disabled_PassThrough verifies that when cc.enabled == false,
// DecodeHeaders returns Continue AND clears f.record (false per AMEND-11 — a
// disabled-filter request is NOT classified at encode time). The rqRejected
// counter must NOT increment (the request is passed through, not rejected).
// No SendLocalReply is emitted. Per SPEC §6.4 gate 1 + AMEND-11.
func TestDecodeHeaders_Disabled_PassThrough(t *testing.T) {
	f, _, _, cb := newACTestFilter(t, neverRejectRand())
	f.cc.enabled = false

	rqRejectedBefore := f.stats.rqRejected.Load()

	status := f.DecodeHeaders(http.Header{}, false)

	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (disabled pass-through)", status)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on disabled pass-through; got %+v", cb.localReply)
	}
	if f.record {
		t.Errorf("f.record = true after disabled pass-through; want false (AMEND-11 cleared)")
	}
	if got := f.stats.rqRejected.Load(); got != rqRejectedBefore {
		t.Errorf("rqRejected incremented on disabled pass-through; got %d, want %d (unchanged)", got, rqRejectedBefore)
	}
}

// -----------------------------------------------------------------------------
// 3. TestDecodeHeaders_RpsSuppression
// -----------------------------------------------------------------------------

// TestDecodeHeaders_RpsSuppression verifies that when averageRps() <
// cc.rpsThreshold, DecodeHeaders returns Continue WITHOUT consulting
// shouldReject(), and f.record stays true (the request proceeds to encode where
// it will be classified). Per SPEC §6.4 gate 2.
//
// Setup: window primed with 10 failures (P>0) + alwaysRejectRand ⇒ shouldReject
// WOULD return true if consulted. Clock stays at time.Unix(0,0) so all 10 requests
// land in one bucket; averageRps()=10/30=0 (integer). rpsThreshold=10 ⇒ 0 < 10 ⇒
// suppression fires before shouldReject is ever called ⇒ must still return Continue.
func TestDecodeHeaders_RpsSuppression(t *testing.T) {
	// Use alwaysRejectRand + a primed window (P>0) to prove shouldReject is NOT
	// consulted: if gate 2 (RPS-suppression) failed to short-circuit, gate 3 would
	// see P>0 and r%10000==0 → shouldReject()=true → StopIteration. The test
	// guarantees Continue only because the RPS gate fires first.
	f, ctrl, _, cb := newACTestFilter(t, alwaysRejectRand())
	primeWindowForRejection(t, ctrl) // P>0: shouldReject WOULD reject if consulted
	f.cc.rpsThreshold = 10           // averageRps()=10/30=0 < 10 ⇒ suppression

	rqRejectedBefore := f.stats.rqRejected.Load()

	status := f.DecodeHeaders(http.Header{}, false)

	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (RPS suppression gate)", status)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on RPS suppression; got %+v", cb.localReply)
	}
	if !f.record {
		t.Errorf("f.record = false after RPS suppression; want true (request proceeds to encode)")
	}
	if got := f.stats.rqRejected.Load(); got != rqRejectedBefore {
		t.Errorf("rqRejected incremented on RPS suppression; got %d, want %d (unchanged)", got, rqRejectedBefore)
	}
}

// -----------------------------------------------------------------------------
// 4. TestDecodeHeaders_Reject_Increments_rqRejected
// -----------------------------------------------------------------------------

// TestDecodeHeaders_Reject_Increments_rqRejected verifies that when shouldReject()
// returns true (via injected alwaysRejectRand + primed window), the rqRejected
// counter increments exactly once and f.record is cleared (false per AMEND-11).
// Per SPEC §6.4 gate 3 + AMEND-11.
func TestDecodeHeaders_Reject_Increments_rqRejected(t *testing.T) {
	f, ctrl, _, _ := newACTestFilter(t, alwaysRejectRand())
	f.cc.rpsThreshold = 0 // ensure RPS gate passes (empty window ⇒ averageRps=0 < 0 is false)
	primeWindowForRejection(t, ctrl)

	rqRejectedBefore := f.stats.rqRejected.Load()

	status := f.DecodeHeaders(http.Header{}, false)

	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration (reject gate)", status)
	}
	if f.record {
		t.Errorf("f.record = true after reject; want false (AMEND-11 cleared on reject)")
	}
	if got := f.stats.rqRejected.Load(); got != rqRejectedBefore+1 {
		t.Errorf("rqRejected: got %d, want %d (single increment per rejected request)", got, rqRejectedBefore+1)
	}
}

// -----------------------------------------------------------------------------
// 5. TestDecodeHeaders_Reject_SendLocalReply_503_EmptyBody
// -----------------------------------------------------------------------------

// TestDecodeHeaders_Reject_SendLocalReply_503_EmptyBody verifies the reject wire
// shape per AMEND-7 + PD-2.503: status 503, empty body "", nil headers.
//
// The "denied_by_admission_control" rc-details is NOT surfaceable through the
// 3-arg SendLocalReply API (ABSENT-by-API per PD-2.503) — NOT pinned here.
func TestDecodeHeaders_Reject_SendLocalReply_503_EmptyBody(t *testing.T) {
	f, ctrl, _, cb := newACTestFilter(t, alwaysRejectRand())
	f.cc.rpsThreshold = 0
	primeWindowForRejection(t, ctrl)

	status := f.DecodeHeaders(http.Header{}, false)

	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration (reject gate)", status)
	}
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want invocation on reject; got nil")
	}
	// AMEND-7 + PD-2.503 byte-pinned reject wire shape.
	if cb.localReply.status != 503 {
		t.Errorf("SendLocalReply status: got %d, want 503 (AMEND-7 + PD-2.503)", cb.localReply.status)
	}
	if cb.localReply.body != "" {
		t.Errorf("SendLocalReply body: got %q, want %q (AMEND-7 + PD-2.503 empty body)", cb.localReply.body, "")
	}
	if cb.localReply.headers != nil {
		t.Errorf("SendLocalReply headers: got %v, want nil (PD-2.503 no added headers)", cb.localReply.headers)
	}
}

// -----------------------------------------------------------------------------
// 6. TestDecodeHeaders_Admit_PassThrough
// -----------------------------------------------------------------------------

// TestDecodeHeaders_Admit_PassThrough verifies that when shouldReject() returns
// false (via injected neverRejectRand + primed window with enough successes that
// P is clamped to 0 by the formula), DecodeHeaders returns Continue and f.record
// stays true (the request proceeds to encode and will be classified).
// Per SPEC §6.4 gate 3 → admit → gate 4 (pass-through).
//
// Setup: empty window ⇒ P = inner = (0 - 0/0.95)/(0+1) = 0 ⇒ never reject.
func TestDecodeHeaders_Admit_PassThrough(t *testing.T) {
	f, _, _, cb := newACTestFilter(t, neverRejectRand())
	f.cc.rpsThreshold = 0 // gate 2 passes (0 < 0 is false → not suppressed)

	rqRejectedBefore := f.stats.rqRejected.Load()

	status := f.DecodeHeaders(http.Header{}, false)

	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (shouldReject=false → admit)", status)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on admit; got %+v", cb.localReply)
	}
	if !f.record {
		t.Errorf("f.record = false after admit; want true (request proceeds to encode)")
	}
	if got := f.stats.rqRejected.Load(); got != rqRejectedBefore {
		t.Errorf("rqRejected incremented on admit; got %d, want %d (unchanged)", got, rqRejectedBefore)
	}
}
