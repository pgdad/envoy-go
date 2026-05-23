package ratelimit

// decode_headers_test.go — TDD coverage for the §4.6 decode-side dispatch
// flow per phase-24.1 PLAN Task 7 Step 1. Three required tests:
//
//  1. TestDecodeHeaders_ZeroDescriptors_Continue — when both route- and vhost-
//     level rate_limits are empty (or every policy drops), DecodeHeaders
//     returns Continue WITHOUT consulting the RLS.
//
//  2. TestDecodeHeaders_AsyncDispatch_StopIteration — when at least one
//     descriptor is built, DecodeHeaders returns StopIteration AND the async
//     dispatch goroutine invokes the captured rlsCallFn exactly once.
//
//  3. TestDecodeHeaders_OnDestroy_Cancels — OnDestroy cancels the in-flight
//     callCtx so a parked rlsCallFn observes context.Canceled (the ext_authz
//     callCancel precedent at extauthz.go OnDestroy).
//
// Test-double strategy: production wires a real *grpcclient.RateLimitClient
// through buildCompiledConfig's cluster-load gate; tests inject a synthetic
// `rlsCallFn` directly into the compiledConfig to bypass the gRPC layer (the
// ext_authz `checkFn` precedent at extauthz.go:54).

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratelimitfilterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	ratelimitservicev3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"
	"google.golang.org/protobuf/proto"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// ----------------------------------------------------------------------------
// fakeRatelimitDCB — minimal DecoderFilterCallbacks double per the ext_authz
// fakeExtAuthzDCB precedent at extauthz_test.go:3562.
// ----------------------------------------------------------------------------

type fakeRatelimitDCB struct {
	mu sync.Mutex

	// Seeds for the §4.3 + ADR-0198 accessor pair.
	routeRLs []*routev3.RateLimit
	vhostRLs []*routev3.RateLimit
	remote   net.Addr

	// Phase 24.2 Task 4 (D-RL11) seeds for the Axis-A + Axis-B walker:
	//   - perRouteCfg: the per-route RateLimitPerRoute proto.Message exposed via
	//     RequestRouteConfig() (typed_per_filter_config resolution result). nil
	//     when no per-route config is present.
	//   - routeIncludeVh: the matched route's legacy
	//     RouteAction.include_vh_rate_limits bool (the chain-seeded value).
	perRouteCfg    proto.Message
	routeIncludeVh bool

	// Call recording.
	continueDecodingCount int
	localReplyCount       int
	localReplyArgs        localReplyArgs
}

type localReplyArgs struct {
	status  int
	body    string
	headers envoyhttp.OrderedHeaders
}

func newFakeRatelimitDCB() *fakeRatelimitDCB {
	return &fakeRatelimitDCB{
		remote: &net.TCPAddr{IP: net.IPv4(203, 0, 113, 42), Port: 51234},
	}
}

func (c *fakeRatelimitDCB) ContinueDecoding() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.continueDecodingCount++
}

func (c *fakeRatelimitDCB) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.localReplyCount++
	c.localReplyArgs = localReplyArgs{status: status, body: body, headers: headers}
}

func (c *fakeRatelimitDCB) RequestRouteConfig() proto.Message           { return c.perRouteCfg }
func (c *fakeRatelimitDCB) RouteRateLimits() []*routev3.RateLimit       { return c.routeRLs }
func (c *fakeRatelimitDCB) VirtualHostRateLimits() []*routev3.RateLimit { return c.vhostRLs }
func (c *fakeRatelimitDCB) RouteMetadata() *corev3.Metadata             { return nil }
func (c *fakeRatelimitDCB) RouteIncludeVhRateLimits() bool              { return c.routeIncludeVh }
func (c *fakeRatelimitDCB) DownstreamRemoteAddr() net.Addr              { return c.remote }
func (c *fakeRatelimitDCB) DownstreamLocalAddr() net.Addr               { return nil }
func (c *fakeRatelimitDCB) DownstreamTLSServerName() string             { return "" }
func (c *fakeRatelimitDCB) DownstreamTLSPeerCertDER() []byte            { return nil }
func (c *fakeRatelimitDCB) DownstreamProtocol() string                  { return "" }
func (c *fakeRatelimitDCB) ListenerPrincipal() string                   { return "" }
func (c *fakeRatelimitDCB) DownstreamPrincipal() []string               { return nil }
func (c *fakeRatelimitDCB) DownstreamTLSConnectionState() *tls.ConnectionState {
	return nil
}
func (c *fakeRatelimitDCB) DynamicMetadata() *dynamicmetadata.Bucket { return nil }
func (c *fakeRatelimitDCB) EncodeHeaders(_ http.Header, _ bool)      {}
func (c *fakeRatelimitDCB) EncodeData(_ []byte, _ bool)              {}
func (c *fakeRatelimitDCB) EncodeTrailers(_ http.Header)             {}
func (c *fakeRatelimitDCB) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}

// snapshotLocalReply returns a copy of the most recent SendLocalReply args under lock.
func (c *fakeRatelimitDCB) snapshotLocalReply() (int, localReplyArgs) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.localReplyCount, c.localReplyArgs
}

func (c *fakeRatelimitDCB) snapshotContinueCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.continueDecodingCount
}

// Compile-time assertion: fakeRatelimitDCB satisfies DecoderFilterCallbacks.
var _ envoyhttp.DecoderFilterCallbacks = (*fakeRatelimitDCB)(nil)

// ----------------------------------------------------------------------------
// fakeRLSCall — scripted rlsCallFn (the test seam mirroring ext_authz checkFn).
// ----------------------------------------------------------------------------

type fakeRLSCall struct {
	mu     sync.Mutex
	called int32
	resp   *ratelimitservicev3.RateLimitResponse
	err    error

	// When non-nil, the goroutine blocks on a select of {<-block, <-ctx.Done()}
	// so OnDestroy tests can drive the ctx.Done branch.
	block chan struct{}

	// Records ctx.Err() at call return.
	lastCtxErr error

	// Records the request (for assertion on domain / descriptors).
	lastReq *ratelimitservicev3.RateLimitRequest
}

func (f *fakeRLSCall) fn(ctx context.Context, req *ratelimitservicev3.RateLimitRequest) (*ratelimitservicev3.RateLimitResponse, error) {
	atomic.AddInt32(&f.called, 1)
	f.mu.Lock()
	f.lastReq = req
	f.mu.Unlock()
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			f.mu.Lock()
			f.lastCtxErr = ctx.Err()
			f.mu.Unlock()
			return nil, ctx.Err()
		}
	}
	return f.resp, f.err
}

func (f *fakeRLSCall) callCount() int { return int(atomic.LoadInt32(&f.called)) }

// ----------------------------------------------------------------------------
// Test-only filter constructor
// ----------------------------------------------------------------------------

// newTestFilter builds a *filter with a synthetic compiledConfig + the
// provided rlsCallFn injected. cc.stats is ensured non-nil but with nil-
// counter fields per ADR-0085 nil-tolerance.
func newTestFilter(t *testing.T, cc *compiledConfig, dcb envoyhttp.DecoderFilterCallbacks) *filter {
	t.Helper()
	if cc.stats == nil {
		cc.stats = &filterStats{} // ADR-0085 nil-counter-tolerant
	}
	return &filter{cc: cc, dcb: dcb}
}

// genericKeyPolicy returns a single-action route policy that always builds
// exactly one descriptor with {generic_key, value}.
func genericKeyPolicy(value string) *routev3.RateLimit {
	return &routev3.RateLimit{Actions: []*routev3.RateLimit_Action{{
		ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
			GenericKey: &routev3.RateLimit_Action_GenericKey{DescriptorValue: value},
		},
	}}}
}

// ----------------------------------------------------------------------------
// Test 1: TestDecodeHeaders_ZeroDescriptors_Continue
// ----------------------------------------------------------------------------

// TestDecodeHeaders_ZeroDescriptors_Continue verifies the zero-descriptor
// short-circuit per parent SPEC §4.6: when no policies are configured (route
// AND vhost both nil/empty), DecodeHeaders returns Continue WITHOUT invoking
// the RLS.
func TestDecodeHeaders_ZeroDescriptors_Continue(t *testing.T) {
	dcb := newFakeRatelimitDCB() // routeRLs + vhostRLs both nil
	call := &fakeRLSCall{
		resp: &ratelimitservicev3.RateLimitResponse{OverallCode: ratelimitservicev3.RateLimitResponse_OK},
	}
	cc := &compiledConfig{
		domain:    "test",
		rlsCallFn: call.fn,
	}
	f := newTestFilter(t, cc, dcb)

	status := f.DecodeHeaders(http.Header{}, true /* endStream */)

	if status != envoyhttp.Continue {
		t.Errorf("DecodeHeaders: got %v, want Continue", status)
	}
	if call.callCount() != 0 {
		t.Errorf("rlsCallFn calls: got %d, want 0 (zero-descriptor short-circuit)", call.callCount())
	}
	if dcb.snapshotContinueCount() != 0 {
		t.Errorf("ContinueDecoding: got %d, want 0 (no async dispatch ⇒ no resume)", dcb.snapshotContinueCount())
	}
}

// ----------------------------------------------------------------------------
// Test 2: TestDecodeHeaders_AsyncDispatch_StopIteration
// ----------------------------------------------------------------------------

// TestDecodeHeaders_AsyncDispatch_StopIteration verifies the non-empty-
// descriptor async-dispatch path per parent SPEC §4.6:
//
//  1. DecodeHeaders returns StopIteration synchronously.
//  2. The async goroutine invokes rlsCallFn exactly once.
//  3. On OK disposition the goroutine wakes the parked chain via
//     ContinueDecoding (NO SendLocalReply).
//  4. The request envelope carries the filter's domain.
func TestDecodeHeaders_AsyncDispatch_StopIteration(t *testing.T) {
	dcb := newFakeRatelimitDCB()
	dcb.routeRLs = []*routev3.RateLimit{genericKeyPolicy("foo")}
	call := &fakeRLSCall{
		resp: &ratelimitservicev3.RateLimitResponse{OverallCode: ratelimitservicev3.RateLimitResponse_OK},
	}
	cc := &compiledConfig{
		domain:    "my-domain",
		rlsCallFn: call.fn,
	}
	f := newTestFilter(t, cc, dcb)

	status := f.DecodeHeaders(http.Header{}, true)

	if status != envoyhttp.StopIteration {
		t.Fatalf("DecodeHeaders: got %v, want StopIteration", status)
	}

	waitForFn(t, func() bool { return call.callCount() >= 1 }, time.Second)
	waitForFn(t, func() bool { return dcb.snapshotContinueCount() >= 1 }, time.Second)

	if got := call.callCount(); got != 1 {
		t.Errorf("rlsCallFn calls: got %d, want 1", got)
	}
	if got := dcb.snapshotContinueCount(); got != 1 {
		t.Errorf("ContinueDecoding: got %d, want 1 (OK disposition resumes the chain)", got)
	}
	count, _ := dcb.snapshotLocalReply()
	if count != 0 {
		t.Errorf("SendLocalReply: got %d, want 0 (OK disposition does NOT short-circuit)", count)
	}

	call.mu.Lock()
	req := call.lastReq
	call.mu.Unlock()
	if req == nil {
		t.Fatal("rlsCallFn req: got nil, want non-nil")
	}
	if got := req.GetDomain(); got != "my-domain" {
		t.Errorf("rlsCallFn req.Domain: got %q, want %q", got, "my-domain")
	}
	if got := len(req.GetDescriptors()); got != 1 {
		t.Errorf("rlsCallFn req.Descriptors: got %d, want 1 (one generic_key policy)", got)
	}
}

// ----------------------------------------------------------------------------
// Test 3: TestDecodeHeaders_OnDestroy_Cancels
// ----------------------------------------------------------------------------

// TestDecodeHeaders_OnDestroy_Cancels verifies the OnDestroy cancellation path
// per the ext_authz callCancel precedent: when OnDestroy fires while the
// rlsCallFn is in flight, the captured callCtx is canceled, the in-flight
// goroutine observes context.Canceled, AND the f.done guard suppresses the
// disposition apply (no dcb mutation after OnDestroy).
func TestDecodeHeaders_OnDestroy_Cancels(t *testing.T) {
	dcb := newFakeRatelimitDCB()
	dcb.routeRLs = []*routev3.RateLimit{genericKeyPolicy("foo")}
	call := &fakeRLSCall{
		block: make(chan struct{}), // never closed — only ctx.Done can unblock
	}
	cc := &compiledConfig{domain: "test", rlsCallFn: call.fn}
	f := newTestFilter(t, cc, dcb)

	status := f.DecodeHeaders(http.Header{}, true)
	if status != envoyhttp.StopIteration {
		t.Fatalf("DecodeHeaders: got %v, want StopIteration", status)
	}

	// Spin until the goroutine has entered the call.
	waitForFn(t, func() bool { return call.callCount() >= 1 }, time.Second)

	f.OnDestroy()

	waitForFn(t, func() bool {
		call.mu.Lock()
		defer call.mu.Unlock()
		return call.lastCtxErr == context.Canceled
	}, time.Second)

	call.mu.Lock()
	gotErr := call.lastCtxErr
	call.mu.Unlock()
	if gotErr != context.Canceled {
		t.Errorf("rlsCallFn ctx.Err(): got %v, want context.Canceled", gotErr)
	}

	if got := dcb.snapshotContinueCount(); got != 0 {
		t.Errorf("ContinueDecoding after OnDestroy: got %d, want 0 (done guard suppresses dispatch)", got)
	}
	count, _ := dcb.snapshotLocalReply()
	if count != 0 {
		t.Errorf("SendLocalReply after OnDestroy: got %d, want 0", count)
	}
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// waitForFn polls fn() until it returns true OR the deadline elapses. Fails
// the test on timeout. Polling cadence: 1ms.
func waitForFn(t *testing.T, fn func() bool, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("waitForFn: timeout after %v", within)
}

// ----------------------------------------------------------------------------
// Phase 24.2 Task 4 (D-RL11) — decode_headers.go integration tests.
// ----------------------------------------------------------------------------

// TestDecodeHeaders_PerRoute_Domain verifies that a non-empty per-route
// `RateLimitPerRoute.domain` overrides the filter-config domain in the
// outbound RateLimitRequest envelope (D-RL11 wins-discipline). When the
// per-route domain is empty, the filter-config domain is used.
func TestDecodeHeaders_PerRoute_Domain(t *testing.T) {
	t.Run("PerRouteDomainOverrides", func(t *testing.T) {
		dcb := newFakeRatelimitDCB()
		dcb.routeRLs = []*routev3.RateLimit{genericKeyPolicy("foo")}
		// Per-route config with a non-empty domain override.
		dcb.perRouteCfg = &ratelimitfilterv3.RateLimitPerRoute{
			Domain: "per-route-domain",
		}
		call := &fakeRLSCall{
			resp: &ratelimitservicev3.RateLimitResponse{OverallCode: ratelimitservicev3.RateLimitResponse_OK},
		}
		cc := &compiledConfig{
			domain:    "filter-config-domain",
			rlsCallFn: call.fn,
		}
		f := newTestFilter(t, cc, dcb)

		status := f.DecodeHeaders(http.Header{}, true)
		if status != envoyhttp.StopIteration {
			t.Fatalf("DecodeHeaders: got %v, want StopIteration", status)
		}
		waitForFn(t, func() bool { return call.callCount() >= 1 }, time.Second)

		call.mu.Lock()
		req := call.lastReq
		call.mu.Unlock()
		if req == nil {
			t.Fatal("rlsCallFn req: got nil, want non-nil")
		}
		if got := req.GetDomain(); got != "per-route-domain" {
			t.Errorf("rlsCallFn req.Domain: got %q, want %q (per-route override)", got, "per-route-domain")
		}
	})

	t.Run("EmptyPerRouteDomainFallsBackToFilterConfig", func(t *testing.T) {
		dcb := newFakeRatelimitDCB()
		dcb.routeRLs = []*routev3.RateLimit{genericKeyPolicy("foo")}
		// Per-route config WITHOUT a domain override.
		dcb.perRouteCfg = &ratelimitfilterv3.RateLimitPerRoute{
			Domain: "",
		}
		call := &fakeRLSCall{
			resp: &ratelimitservicev3.RateLimitResponse{OverallCode: ratelimitservicev3.RateLimitResponse_OK},
		}
		cc := &compiledConfig{
			domain:    "filter-config-domain",
			rlsCallFn: call.fn,
		}
		f := newTestFilter(t, cc, dcb)

		status := f.DecodeHeaders(http.Header{}, true)
		if status != envoyhttp.StopIteration {
			t.Fatalf("DecodeHeaders: got %v, want StopIteration", status)
		}
		waitForFn(t, func() bool { return call.callCount() >= 1 }, time.Second)

		call.mu.Lock()
		req := call.lastReq
		call.mu.Unlock()
		if req == nil {
			t.Fatal("rlsCallFn req: got nil, want non-nil")
		}
		if got := req.GetDomain(); got != "filter-config-domain" {
			t.Errorf("rlsCallFn req.Domain: got %q, want %q (filter-config fallback)", got, "filter-config-domain")
		}
	})

	t.Run("NoPerRouteFallsBackToFilterConfig", func(t *testing.T) {
		dcb := newFakeRatelimitDCB()
		dcb.routeRLs = []*routev3.RateLimit{genericKeyPolicy("foo")}
		// dcb.perRouteCfg left nil (no per-route config).
		call := &fakeRLSCall{
			resp: &ratelimitservicev3.RateLimitResponse{OverallCode: ratelimitservicev3.RateLimitResponse_OK},
		}
		cc := &compiledConfig{
			domain:    "filter-config-domain",
			rlsCallFn: call.fn,
		}
		f := newTestFilter(t, cc, dcb)

		status := f.DecodeHeaders(http.Header{}, true)
		if status != envoyhttp.StopIteration {
			t.Fatalf("DecodeHeaders: got %v, want StopIteration", status)
		}
		waitForFn(t, func() bool { return call.callCount() >= 1 }, time.Second)

		call.mu.Lock()
		req := call.lastReq
		call.mu.Unlock()
		if req == nil {
			t.Fatal("rlsCallFn req: got nil, want non-nil")
		}
		if got := req.GetDomain(); got != "filter-config-domain" {
			t.Errorf("rlsCallFn req.Domain: got %q, want %q (nil per-route fallback)", got, "filter-config-domain")
		}
	})
}

// TestDecodeHeaders_PerRoute_AxisAEarlyReturn verifies the D-RL11 Axis-A
// early-return: when the per-route `rate_limits[]` is non-empty, the walker
// emits descriptors ONLY for that list — route-table + vhost-table policies
// passed via RouteRateLimits()/VirtualHostRateLimits() are bypassed entirely.
func TestDecodeHeaders_PerRoute_AxisAEarlyReturn(t *testing.T) {
	dcb := newFakeRatelimitDCB()
	// Route + vhost tables present — must be IGNORED under Axis-A.
	dcb.routeRLs = []*routev3.RateLimit{genericKeyPolicy("rt_table")}
	dcb.vhostRLs = []*routev3.RateLimit{genericKeyPolicy("vh_table")}
	// Per-route Axis-A list — walker MUST emit ONLY this.
	dcb.perRouteCfg = &ratelimitfilterv3.RateLimitPerRoute{
		RateLimits: []*routev3.RateLimit{genericKeyPolicy("pr_axisA")},
	}
	call := &fakeRLSCall{
		resp: &ratelimitservicev3.RateLimitResponse{OverallCode: ratelimitservicev3.RateLimitResponse_OK},
	}
	cc := &compiledConfig{
		domain:    "d",
		rlsCallFn: call.fn,
	}
	f := newTestFilter(t, cc, dcb)

	status := f.DecodeHeaders(http.Header{}, true)
	if status != envoyhttp.StopIteration {
		t.Fatalf("DecodeHeaders: got %v, want StopIteration", status)
	}
	waitForFn(t, func() bool { return call.callCount() >= 1 }, time.Second)

	call.mu.Lock()
	req := call.lastReq
	call.mu.Unlock()
	if req == nil {
		t.Fatal("rlsCallFn req: got nil, want non-nil")
	}
	// Expect EXACTLY 1 descriptor (the per-route policy) — both rt_table
	// AND vh_table must be invisible.
	if got := len(req.GetDescriptors()); got != 1 {
		t.Fatalf("req.Descriptors: got %d, want 1 (Axis-A bypasses route-table + vhost-table)", got)
	}
	d := req.GetDescriptors()[0]
	if len(d.GetEntries()) != 1 {
		t.Fatalf("descriptor entries: got %d, want 1", len(d.GetEntries()))
	}
	e := d.GetEntries()[0]
	// genericKeyPolicy(value) ⇒ descriptor_value=value, descriptor_key default "generic_key".
	if e.GetKey() != "generic_key" || e.GetValue() != "pr_axisA" {
		t.Errorf("descriptor entry: got {%s,%s}, want {generic_key,pr_axisA}", e.GetKey(), e.GetValue())
	}
}

// TestDecodeHeaders_PerRoute_AxisB_LegacyForceInclude verifies the AMEND-5
// legacy-bool override: when `RouteAction.include_vh_rate_limits=true` is
// seeded via RouteIncludeVhRateLimits(), the vhost table is walked even when
// the per-route `vh_rate_limits` enum is IGNORE.
func TestDecodeHeaders_PerRoute_AxisB_LegacyForceInclude(t *testing.T) {
	dcb := newFakeRatelimitDCB()
	dcb.routeRLs = []*routev3.RateLimit{genericKeyPolicy("rt")}
	dcb.vhostRLs = []*routev3.RateLimit{genericKeyPolicy("vh")}
	// Per-route says IGNORE — vhost should be skipped per the enum.
	dcb.perRouteCfg = &ratelimitfilterv3.RateLimitPerRoute{
		VhRateLimits: ratelimitfilterv3.RateLimitPerRoute_IGNORE,
	}
	// BUT the legacy bool is true — force-include trumps the enum.
	dcb.routeIncludeVh = true

	call := &fakeRLSCall{
		resp: &ratelimitservicev3.RateLimitResponse{OverallCode: ratelimitservicev3.RateLimitResponse_OK},
	}
	cc := &compiledConfig{
		domain:    "d",
		rlsCallFn: call.fn,
	}
	f := newTestFilter(t, cc, dcb)

	status := f.DecodeHeaders(http.Header{}, true)
	if status != envoyhttp.StopIteration {
		t.Fatalf("DecodeHeaders: got %v, want StopIteration", status)
	}
	waitForFn(t, func() bool { return call.callCount() >= 1 }, time.Second)

	call.mu.Lock()
	req := call.lastReq
	call.mu.Unlock()
	if req == nil {
		t.Fatal("rlsCallFn req: got nil, want non-nil")
	}
	// Expect 2 descriptors: route + vhost (legacy force-include trumps IGNORE).
	if got := len(req.GetDescriptors()); got != 2 {
		t.Fatalf("req.Descriptors: got %d, want 2 (legacy include_vh_rate_limits=true forces vhost walk)", got)
	}
}
