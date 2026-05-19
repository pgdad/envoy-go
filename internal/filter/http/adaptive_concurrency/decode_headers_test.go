package adaptive_concurrency

// decode_headers_test.go — Group 6 DecodeHeaders dispatch tests for the
// adaptive_concurrency filter per phase-21 SPEC §6.4 + AMEND-6 + §14.1.
//
// Covers four scenarios:
//
//  1. Disabled pass-through: cc.enabled == false ⇒ Continue without
//     consulting the controller (no rqBlocked increment, no entryTime, no
//     acquired flag flip).
//  2. Forward (capacity available): forwardingDecision returns true ⇒
//     Continue + f.acquired == true + f.entryTime set to clock.Now().
//  3. Block (at capacity): forwardingDecision returns false ⇒
//     SendLocalReply(503, "reached concurrency limit", {content-type:
//     text/plain}) + StopIteration. The 503 wire shape is byte-pinned per
//     AMEND-6 + SPEC §11 §21.P1 — body is the 25-byte verbatim string + the
//     content-type header is lowercase per HCM-fixture convention (SPEC line
//     440).
//  4. Block + custom status: when cc.concurrencyLimitExceededStatus == 429,
//     SendLocalReply receives status=429 (verifying the config knob is
//     honored — default is 503 but operators may override via the
//     concurrency_limit_exceeded_status proto field).
//
// These tests close the Task-3 deferral pin "TestController_503_BodyAndHeaders_
// ByteExact → Task 5" recorded at controller_test.go header + PROGRESS.md
// Task 3 entry.

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// -----------------------------------------------------------------------------
// recordedCallbacks — test-double DecoderFilterCallbacks for SendLocalReply
// capture per the rbac_test.go::rbacFakeCB precedent. The methods unused by
// adaptive_concurrency at decode time (ContinueDecoding, RequestRouteConfig,
// ADR-0165 connection-info accessors) return zero values.
// -----------------------------------------------------------------------------

// localReplyArgs captures one SendLocalReply invocation.
type localReplyArgs struct {
	status  int
	body    string
	headers envoyhttp.OrderedHeaders
}

// recordedCallbacks is a test-double DecoderFilterCallbacks that records
// SendLocalReply invocations. Mirrors rbac_test.go::rbacFakeCB; adapted for
// adaptive_concurrency's narrower decode-time consumption (no per-route
// config; no DownstreamPrincipal).
type recordedCallbacks struct {
	mu         sync.Mutex
	localReply *localReplyArgs // captured at SendLocalReply; nil if never called
}

func (c *recordedCallbacks) ContinueDecoding() {}

func (c *recordedCallbacks) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.localReply = &localReplyArgs{status: status, body: body, headers: headers}
}

func (c *recordedCallbacks) RequestRouteConfig() proto.Message { return nil }
func (c *recordedCallbacks) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (c *recordedCallbacks) EncodeHeaders(http.Header, bool) {}
func (c *recordedCallbacks) EncodeData([]byte, bool)         {}
func (c *recordedCallbacks) EncodeTrailers(http.Header)      {}
func (c *recordedCallbacks) DownstreamPrincipal() []string   { return nil }

// ADR-0165 callback-surface extension stubs (phase-18.2 Task 4).
func (c *recordedCallbacks) DownstreamRemoteAddr() net.Addr   { return nil }
func (c *recordedCallbacks) DownstreamLocalAddr() net.Addr    { return nil }
func (c *recordedCallbacks) DownstreamTLSServerName() string  { return "" }
func (c *recordedCallbacks) DownstreamTLSPeerCertDER() []byte { return nil }
func (c *recordedCallbacks) DownstreamProtocol() string       { return "" }
func (c *recordedCallbacks) ListenerPrincipal() string        { return "" }

// ADR-0192 callback-surface extension stubs (phase-22.2 Task 5).
func (c *recordedCallbacks) DownstreamTLSConnectionState() *tls.ConnectionState { return nil }
func (c *recordedCallbacks) DynamicMetadata() *dynamicmetadata.Bucket           { return nil }

// -----------------------------------------------------------------------------
// Test-scope filter helpers
// -----------------------------------------------------------------------------

// newTestFilter constructs a *filter wired to a fresh *gradientController +
// fakeClock. The returned filter's cc.enabled is true by default; tests that
// need the disabled path override cc.enabled before invoking DecodeHeaders.
//
// Mirrors newTestController at controller_test.go (same cfg + same clock
// anchor at time.Unix(0, 0)).
func newTestFilter(t *testing.T) (*filter, *fakeClock, *recordedCallbacks) {
	t.Helper()
	cfg := testCompiledConfig()
	clock := newFakeClock(time.Unix(0, 0))
	ctrl := newGradientController(cfg, testFilterStats(), clock)
	cb := &recordedCallbacks{}
	f := &filter{
		cc:         cfg,
		controller: ctrl,
		clock:      clock,
	}
	f.SetDecoderCallbacks(cb)
	return f, clock, cb
}

// -----------------------------------------------------------------------------
// Test 1: Disabled pass-through
// -----------------------------------------------------------------------------

// TestFilter_DecodeHeaders_Disabled_PassThrough verifies that when
// cc.enabled == false, DecodeHeaders returns Continue WITHOUT consulting the
// controller (the rqBlocked counter does NOT increment + the acquired flag
// stays false + entryTime stays zero). Per SPEC §6.4 disabled-leg.
func TestFilter_DecodeHeaders_Disabled_PassThrough(t *testing.T) {
	f, _, cb := newTestFilter(t)
	f.cc.enabled = false

	rqBlockedBefore := f.controller.stats.rqBlocked.Load()
	status := f.DecodeHeaders(http.Header{}, false)

	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (disabled pass-through)", status)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on disabled pass-through; got %+v", cb.localReply)
	}
	if f.acquired {
		t.Errorf("f.acquired = true on disabled pass-through; want false (controller not consulted)")
	}
	if !f.entryTime.IsZero() {
		t.Errorf("f.entryTime = %v on disabled pass-through; want zero (controller not consulted)", f.entryTime)
	}
	if got := f.controller.stats.rqBlocked.Load(); got != rqBlockedBefore {
		t.Errorf("rqBlocked counter incremented on disabled pass-through; got %d, want %d (unchanged)", got, rqBlockedBefore)
	}
}

// -----------------------------------------------------------------------------
// Test 2: Forward (capacity available)
// -----------------------------------------------------------------------------

// TestFilter_DecodeHeaders_Forward_AcquiresToken verifies that when the
// controller has capacity (numRqOutstanding < concurrencyLimit), DecodeHeaders
// returns Continue + sets f.acquired = true + records f.entryTime = clock.Now().
// Per SPEC §6.4 Forward leg.
func TestFilter_DecodeHeaders_Forward_AcquiresToken(t *testing.T) {
	f, clock, cb := newTestFilter(t)
	// At construction, limit = minConcurrency = 3 + numRqOutstanding = 0 ⇒
	// capacity available.
	expectedEntryTime := clock.Now()

	status := f.DecodeHeaders(http.Header{}, false)

	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (Forward — capacity available)", status)
	}
	if cb.localReply != nil {
		t.Errorf("SendLocalReply must NOT fire on Forward; got %+v", cb.localReply)
	}
	if !f.acquired {
		t.Errorf("f.acquired = false after Forward; want true (token acquired)")
	}
	if !f.entryTime.Equal(expectedEntryTime) {
		t.Errorf("f.entryTime = %v; want %v (clock.Now() at Forward)", f.entryTime, expectedEntryTime)
	}
	if got := f.controller.numRqOutstanding.Load(); got != 1 {
		t.Errorf("numRqOutstanding = %d after Forward; want 1 (CAS incremented)", got)
	}
}

// -----------------------------------------------------------------------------
// Test 3: Block — 503 wire shape byte-exact (AMEND-6 + SPEC §11 §21.P1)
// -----------------------------------------------------------------------------

// TestFilter_DecodeHeaders_Block_503_BodyAndHeaders_ByteExact verifies that
// when the controller is at capacity (limit=1 + one in-flight already),
// DecodeHeaders emits a 503 with the byte-pinned wire shape per AMEND-6:
//
//   - status: 503
//   - body: "reached concurrency limit" (25 bytes verbatim; no trailing
//     newline; no JSON wrapping)
//   - headers: contains "content-type: text/plain" (lowercase per SPEC line
//     440 fixture convention; HCM auto-injects content-length: 25 downstream)
//
// Closes the Task-3 deferral "TestController_503_BodyAndHeaders_ByteExact →
// Task 5" recorded at controller_test.go header + PROGRESS.md Task 3 entry.
func TestFilter_DecodeHeaders_Block_503_BodyAndHeaders_ByteExact(t *testing.T) {
	f, _, cb := newTestFilter(t)
	// Force the controller to capacity: clamp limit to 1, consume the 1 slot.
	f.controller.concurrencyLimit.Store(1)
	f.controller.numRqOutstanding.Store(1)
	rqBlockedBefore := f.controller.stats.rqBlocked.Load()

	status := f.DecodeHeaders(http.Header{}, false)

	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration (Block leg)", status)
	}
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want invocation on Block; got nil")
	}
	if cb.localReply.status != 503 {
		t.Errorf("SendLocalReply status: got %d, want 503 (default concurrency_limit_exceeded_status)", cb.localReply.status)
	}
	// Body byte-exact 25-byte verbatim per AMEND-6.
	wantBody := "reached concurrency limit"
	if cb.localReply.body != wantBody {
		t.Errorf("SendLocalReply body: got %q, want %q (AMEND-6 byte-pinned)", cb.localReply.body, wantBody)
	}
	if got := len(cb.localReply.body); got != 25 {
		t.Errorf("SendLocalReply body length: got %d, want 25 bytes (AMEND-6 byte-pinned; no trailing newline)", got)
	}
	// Header: content-type: text/plain (lowercase per SPEC line 440 fixture).
	if got := cb.localReply.headers.Get("content-type"); got != "text/plain" {
		t.Errorf("SendLocalReply content-type: got %q, want %q (AMEND-6 byte-pinned)", got, "text/plain")
	}
	// Per-task pin: do NOT increment rqBlocked at the filter; the controller's
	// forwardingDecision() already does. Verify the counter did NOT double-
	// increment.
	if got := f.controller.stats.rqBlocked.Load(); got != rqBlockedBefore+1 {
		t.Errorf("rqBlocked: got %d, want %d (single increment by controller; filter must NOT double-increment)", got, rqBlockedBefore+1)
	}
	// f.acquired must remain false on Block (no token was acquired).
	if f.acquired {
		t.Errorf("f.acquired = true on Block; want false (no token acquired)")
	}
}

// -----------------------------------------------------------------------------
// Test 4: Block — custom concurrency_limit_exceeded_status honored
// -----------------------------------------------------------------------------

// TestFilter_DecodeHeaders_Block_CustomStatus verifies that when the
// concurrency_limit_exceeded_status config knob is set (e.g., 429 Too Many
// Requests), the SendLocalReply status reflects the configured value (not
// the default 503). Documents that the default is 503 (covered by the
// byte-exact test above) but the proto knob is honored.
func TestFilter_DecodeHeaders_Block_CustomStatus(t *testing.T) {
	f, _, cb := newTestFilter(t)
	f.cc.concurrencyLimitExceededStatus = 429
	// Force the controller to capacity.
	f.controller.concurrencyLimit.Store(1)
	f.controller.numRqOutstanding.Store(1)

	status := f.DecodeHeaders(http.Header{}, false)

	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration (Block leg)", status)
	}
	if cb.localReply == nil {
		t.Fatal("SendLocalReply: want invocation on Block; got nil")
	}
	if cb.localReply.status != 429 {
		t.Errorf("SendLocalReply status: got %d, want 429 (custom concurrency_limit_exceeded_status honored)", cb.localReply.status)
	}
	// Body + content-type still byte-pinned (AMEND-6 wire shape applies for
	// any concurrency_limit_exceeded_status value).
	if cb.localReply.body != "reached concurrency limit" {
		t.Errorf("SendLocalReply body: got %q, want %q (AMEND-6 wire shape regardless of status code)", cb.localReply.body, "reached concurrency limit")
	}
}
