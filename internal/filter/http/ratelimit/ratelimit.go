package ratelimit

// ratelimit.go — TypeURL + filterName constants + the per-stream filter
// struct + compile-time interface assertions + the FULL `New`
// HTTPFilterFactory body (Task 7).
//
// # Filter shape (both-sides)
//
// The per-stream *filter implements BOTH StreamDecoderFilter AND
// StreamEncoderFilter per parent SPEC §6.5/§6.6. The SAME *filter instance
// is wired as `HTTPFilter{Decoder: f, Encoder: f}` at Task-7's factory
// return (the admission_control / cors precedent). At 24.1 the encode-side
// is a STUB returning Continue per D-RL7 — the X-RateLimit DRAFT_VERSION_03
// header injection lands at 24.2 (parent §6.6).
//
// # Async-dispatch shape per parent SPEC §4.6 + the ext_authz precedent
//
//   - DecodeHeaders (decode_headers.go) reads the route + vhost rate_limits
//     accessors per ADR-0198 (DELTA-2), builds descriptors via the §4
//     engine (descriptors.go), and either returns Continue (zero descriptors)
//     OR fires the async ShouldRateLimit + returns StopIteration.
//   - The async goroutine invokes the captured rlsCallFn under a per-stream
//     cancellable context (callCtx/callCancel pair; the ext_authz
//     extauthz.go:1037 precedent).
//   - On goroutine return, applyDisposition (dispositions.go) acquires f.mu
//     + checks f.done; on done==false applies the OK/OVER_LIMIT/error
//     disposition + wakes the parked chain via dcb.ContinueDecoding() OR
//     dcb.SendLocalReply(...).
//   - OnDestroy sets f.done=true under f.mu + cancels callCtx (the
//     resume-after-OnDestroy guard per the ext_authz pattern at
//     extauthz.go:1343).
//
// # Cross-references
//
//   - ADR-0070 (filter-registration convention)
//   - ADR-0071 (two-step factory; HTTPFilterFactory)
//   - ADR-0072 (boot-time-fail-fast)
//   - ADR-0085 (nil-tolerance for ctx.Stats)
//   - ADR-0143 SN1 (byte-exact TypeURL pin)
//   - ADR-0158 (grpcclient two-tier discipline; *RateLimitClient is the
//     third typed wrapper — DELTA-1 at Task 2)
//   - ADR-0197 (CORE slice landed at this task)
//   - parent SPEC §6.1/§6.5/§6.6 + §4.6 + §4.7
//   - internal/filter/http/extauthz/extauthz.go (callCancel + async-dispatch
//     + mu/done guard precedent)

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	ratelimitservicev3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/grpcclient"
)

// filterName is the registered name for the ratelimit filter per ADR-0070
// + parent SPEC §3.4 boot-registration. Matches the listener config
// `http_filters[].name` string + the HCM-chain dispatch identifier.
const filterName = "envoy.filters.http.ratelimit"

// TypeURL is the proto typed_config TypeURL for the external-gRPC global
// rate-limit filter per ADR-0143 SN1 + the v1.32.4 / v1.37.x proto package
// envoy.extensions.filters.http.ratelimit.v3.RateLimit. Byte-exact constant;
// regression-pinned at TestTypeURL.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimit"

// filter is the per-stream ratelimit filter instance per parent SPEC §6 +
// the Task-7 wiring.
//
// Both-sides filter: implements StreamDecoderFilter (DecodeHeaders /
// DecodeData / DecodeTrailers / SetDecoderCallbacks / OnDestroy —
// decode_headers.go + dispositions.go) AND StreamEncoderFilter
// (EncodeHeaders / EncodeData / EncodeTrailers / SetEncoderCallbacks;
// STUBBED at 24.1 below; full X-RateLimit injection at 24.2 per D-RL7).
//
// Per-stream state fields:
//
//   - cc            — shared compiled filter config (per-listener; immutable
//     after New-factory closure capture).
//   - dcb / ecb     — framework-supplied callbacks (SetDecoderCallbacks /
//     SetEncoderCallbacks). The decode-side accessors
//     (RouteRateLimits, VirtualHostRateLimits, DownstreamRemoteAddr)
//     flow through dcb.
//   - callCtx /
//     callCancel    — per-stream cancellable context for the async
//     ShouldRateLimit call. OnDestroy fires callCancel so the in-flight
//     goroutine observes ctx.Err() and returns. The ext_authz precedent
//     at extauthz.go:1037.
//   - mu / done     — the resume-after-OnDestroy race guard. OnDestroy
//     sets done=true under mu + fires callCancel; the resume goroutine
//     acquires mu + checks done before touching dcb (the ext_authz
//     precedent at extauthz.go:1048/1343).
type filter struct {
	cc  *compiledConfig
	ecb envoyhttp.EncoderFilterCallbacks
	dcb envoyhttp.DecoderFilterCallbacks

	// Async-dispatch state per planner-time decision + the ext_authz precedent.
	callCtx    context.Context //nolint:containedctx // stored for OnDestroy cancellation per the ext_authz callCancel precedent (extauthz.go:1037-1039)
	callCancel context.CancelFunc

	// mu + done guard the resume-after-OnDestroy race per the ext_authz
	// precedent (extauthz.go:1048 / 1343).
	mu   sync.Mutex
	done bool

	// responseStatuses is the per-stream X-RateLimit DRAFT_VERSION_03 capture
	// surface (parent SPEC §4.7 + AMEND-8 + D-RL12; ADR-0197 X-RateLimit slice
	// at phase-24.2 Task 5). Stored by dispositions.go on the three "store"
	// arms — OK / OVER_LIMIT / fail-OPEN with a non-nil response — and read by
	// encode.go::encodeHeaders to compute the byte-shape.
	//
	// Lock discipline: WRITTEN under f.mu in dispositions.go::applyDisposition
	// (always under mu — the async-goroutine holds f.mu when it calls
	// applyDisposition; the OVER_LIMIT path's downstream SendLocalReply runs
	// the encode chain synchronously in the same goroutine). READ in
	// encode.go::encodeHeaders WITHOUT acquiring f.mu — re-entrant mu would
	// deadlock the synchronous SendLocalReply→RunEncodeHeaders path. The store
	// completes BEFORE SendLocalReply / ContinueDecoding signals the chain, so
	// happens-before is supplied by the chain dispatch sequencing.
	//
	// fail-CLOSED MUST NOT store per D-RL12 (the 500 path emits no
	// X-RateLimit). A nil/empty responseStatuses on read makes encodeHeaders a
	// clean no-op.
	responseStatuses []*ratelimitservicev3.RateLimitResponse_DescriptorStatus
}

// Static interface conformance assertions. The *filter implements BOTH the
// decoder and encoder side per parent SPEC §6.5/§6.6 + the Task-7
// `HTTPFilter{Decoder: f, Encoder: f}` wiring. The encode-side is a STUB at
// 24.1 per D-RL7 (X-RateLimit DRAFT_VERSION_03 emission lands at 24.2).
var (
	_ envoyhttp.StreamDecoderFilter = (*filter)(nil)
	_ envoyhttp.StreamEncoderFilter = (*filter)(nil)
)

// ----------------------------------------------------------------------------
// New — the FULL HTTPFilterFactory body per Task 7 + parent SPEC §6.1 + the
// ext_authz buildGRPCCheckFn cluster-load + client-construction precedent.
// ----------------------------------------------------------------------------

// New is the HTTPFilterFactory exposed at boot per ADR-0071 + ADR-0072.
//
// Steps (mirrors the ext_authz buildGRPCCheckFn precedent at
// extauthz/check.go:565-574 — cluster-load gates already validated in
// buildCompiledConfig at Task 3; here we construct the typed
// *grpcclient.RateLimitClient AND register the cluster-scoped 4-counter
// stat surface):
//
//  1. typedConfig nil-guard — defensive (buildCompiledConfig also guards but
//     the ADR-0072 boot-time-fail-fast posture wants the first arm here).
//  2. buildCompiledConfig parses + validates the proto envelope; returns
//     *compiledConfig per AMEND-3 13-field roster (Task 3).
//  3. Construct the *grpcclient.RateLimitClient via grpcclient.New(ctx.
//     ClusterManager) + grpcclient.NewRateLimitClient(d, cc.rlsClusterName,
//     cc.timeout) per ADR-0158. Capture into the rlsCallFn closure on cc.
//  4. Register the 4-counter stat surface via newFilterStats(ctx.Stats,
//     cc.rlsClusterName, cc.statPrefix) — nil-tolerant per ADR-0085.
//  5. Return the per-stream FilterInstanceFactory closure. The HTTPFilter
//     value: Decoder=f + Encoder=f (both-sides — the admission_control /
//     cors precedent). 24.1's encode-side is a STUB per D-RL7.
//
// Returns an error from any of:
//   - typedConfig nil
//   - buildCompiledConfig (PARSE-REJECT roster per §5.1 + cluster-load gates)
//   - NewRateLimitClient (dial failure; the cluster is already validated H/2-
//     capable + present at step 2, so this is a real network/credential issue)
func New(typedConfig *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if typedConfig == nil {
		return nil, errors.New("ratelimit: typed_config required")
	}

	cc, err := buildCompiledConfig(typedConfig, ctx)
	if err != nil {
		return nil, err
	}

	// Step 3: construct the *grpcclient.RateLimitClient. The Dialer is
	// goroutine-safe and stateless (per grpcclient.New doc); the
	// RateLimitClient owns the *grpc.ClientConn and is captured into the
	// rlsCallFn closure. Per ADR-0158 §Decision D2 the ClientConn leaks-
	// on-exit at MVP (no Close call from the filter).
	//
	// The ctx.ClusterManager nil-guard already fired inside buildCompiledConfig
	// at the §5.1 arm 10 PARSE-REJECT; reaching here means ClusterManager is
	// non-nil + the cluster is HTTP/2-capable.
	dialer := grpcclient.New(ctx.ClusterManager)
	rlc, err := grpcclient.NewRateLimitClient(dialer, cc.rlsClusterName, cc.timeout)
	if err != nil {
		return nil, fmt.Errorf("ratelimit: rate_limit_service.grpc_service: %w", err)
	}
	cc.rlsCallFn = func(ctx context.Context, req *ratelimitservicev3.RateLimitRequest) (*ratelimitservicev3.RateLimitResponse, error) {
		return rlc.ShouldRateLimit(ctx, req)
	}

	// Step 4: register the cluster-scoped 4-counter stat surface (AMEND-1 +
	// AMEND-10). newFilterStats is nil-tolerant per ADR-0085 — when
	// ctx.Stats is nil it returns an all-nil-counter *filterStats and the
	// dispositions Inc-paths nil-guard each counter.
	cc.stats = newFilterStats(ctx.Stats, cc.rlsClusterName, cc.statPrefix)

	// Step 5: return the per-stream FilterInstanceFactory closure. Both-
	// sides discipline: Decoder == Encoder == the same *filter pointer (the
	// admission_control / cors precedent).
	return func() envoyhttp.HTTPFilter {
		f := &filter{cc: cc}
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: f,
			Encoder: f, // 24.1 STUB per D-RL7; 24.2 lands X-RateLimit injection
		}
	}, nil
}

// ----------------------------------------------------------------------------
// SetDecoderCallbacks / SetEncoderCallbacks
// ----------------------------------------------------------------------------

// SetDecoderCallbacks stores the framework-supplied decoder callbacks per
// ADR-0071. The decode-side reads dcb.RouteRateLimits() +
// dcb.VirtualHostRateLimits() + dcb.DownstreamRemoteAddr() per ADR-0198 +
// ADR-0165 at descriptor-build time.
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }

// SetEncoderCallbacks stores the framework-supplied encoder callbacks per
// ADR-0071. The 24.2 X-RateLimit DRAFT_VERSION_03 emission path mutates the
// response header map directly via the `headers http.Header` argument passed
// to EncodeHeaders (cors precedent at cors.go:138); the ecb is retained for
// any future encode-side primitive that requires callback-side mutation
// (e.g. mid-encode StopIteration → ContinueEncoding park-resume coordination
// — not used at 24.2).
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }

// ----------------------------------------------------------------------------
// Pass-through bodies — decode-side body/trailers + encode-side ALL methods.
// DecodeHeaders lives in decode_headers.go; applyDisposition in dispositions.go.
// ----------------------------------------------------------------------------

// DecodeData is a stub at 24.1; ratelimit operates on the headers phase
// only. The decision is made at DecodeHeaders before body bytes flow.
func (f *filter) DecodeData([]byte, bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// DecodeTrailers is a stub at 24.1; ratelimit operates on the headers
// phase only.
func (f *filter) DecodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// EncodeHeaders dispatches to the per-stream X-RateLimit DRAFT_VERSION_03
// emission body in encode.go::encodeHeaders per phase-24.2 Task 5 (parent
// SPEC §6.6 + AMEND-8 + ADR-0197 X-RateLimit slice). The body reads
// f.responseStatuses (stored by dispositions.go on OK / OVER_LIMIT / fail-
// OPEN), builds the byte-shape per headers.go::buildXRateLimitHeaders, and
// mutates the response header map. No-ops cleanly when
// cc.enableXRateLimitHeaders is false (OFF), when f.cc is nil (test-shape),
// or when responseStatuses is nil/empty (zero-descriptor short-circuit OR
// fail-CLOSED nullptr-mutate D-RL12).
func (f *filter) EncodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	return f.encodeHeaders(headers)
}

// EncodeData is a stub.
func (f *filter) EncodeData([]byte, bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// EncodeTrailers is a stub.
func (f *filter) EncodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// ----------------------------------------------------------------------------
// OnDestroy — the resume-after-OnDestroy guard per the ext_authz precedent.
// ----------------------------------------------------------------------------

// OnDestroy implements the per-stream teardown per parent SPEC §6.5 +
// ADR-0159 async-resume discipline + the ext_authz precedent at
// extauthz.go:1343.
//
//  1. Acquire f.mu and set f.done = true under the lock. This is the
//     resume-after-OnDestroy guard: any resume goroutine that acquires f.mu
//     after OnDestroy observes done=true and aborts the dcb touch without
//     panicking on the already-destroyed stream.
//  2. Capture callCancel under the lock; release; invoke OUTSIDE the lock
//     so the cancel propagation does not run under mu (mirrors
//     extauthz.go:1349 OUTSIDE-the-lock-cancel discipline).
func (f *filter) OnDestroy() {
	f.mu.Lock()
	f.done = true
	cancel := f.callCancel
	f.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}
