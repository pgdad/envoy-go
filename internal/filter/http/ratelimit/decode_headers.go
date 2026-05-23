package ratelimit

// decode_headers.go — the §4.6 decode-side dispatch entry-point per phase-
// 24.1 PLAN Task 7. Reads the chain-seeded route + vhost rate_limits per
// ADR-0198 (DELTA-2 — RouteRateLimits()/VirtualHostRateLimits() accessor
// pair from Task 5), builds descriptors via the §4 engine (descriptors.go;
// Task 6), and either short-circuits to Continue (zero descriptors) OR fires
// the async ShouldRateLimit RPC + parks the chain via StopIteration.
//
// # Dispatch shape (per parent SPEC §4.6 + the ext_authz precedent)
//
//  1. Read RouteRateLimits() + VirtualHostRateLimits() from dcb.
//  2. buildDescriptors(routeRLs, vhostRLs, headers, dcb.DownstreamRemoteAddr(),
//     ""/* clusterName */) — Task 6's pure engine. clusterName is empty at
//     24.1 because the DecoderFilterCallbacks surface has NO
//     MatchedClusterName() accessor at master tip; a config exercising the
//     destination_cluster action at 24.1 produces zero descriptors (the
//     empty-cluster-name whole-descriptor drop arm at
//     descriptors.go::actionDestinationCluster — documented at the engine
//     site). A future framework primitive (MatchedClusterName() accessor)
//     would close this gap; deferred per ADR-0165 narrow-exposure discipline.
//  3. len(descriptors) == 0 ⇒ Continue (no RLS call; zero-regression path).
//  4. Else: build callCtx/callCancel pair (the ext_authz precedent at
//     extauthz.go:1037); store callCancel on f for OnDestroy; spawn the
//     async goroutine; return StopIteration.
//
// # Resume-after-OnDestroy guard (per the ext_authz precedent)
//
// The async goroutine acquires f.mu + checks f.done before invoking
// applyDisposition. OnDestroy sets done=true under mu + fires callCancel
// (extauthz.go:1343). The cancel propagates through the per-stream
// context.WithCancel to the in-flight rlsCallFn; the goroutine returns
// promptly and the f.done check suppresses the disposition apply.
//
// # Cross-references
//
//   - parent SPEC §4.6 (decode-side dispatch + the OK / OVER_LIMIT / error
//     dispositions — bodies in dispositions.go)
//   - ADR-0198 (DELTA-2 RouteRateLimits/VirtualHostRateLimits accessor pair)
//   - ADR-0165 (DownstreamRemoteAddr set-once-by-dispatch accessor)
//   - internal/filter/http/extauthz/extauthz.go:975 dispatchOutboundCheck
//     (the async-dispatch + callCancel precedent)
//   - internal/filter/http/extauthz/extauthz.go:1044 the resume-goroutine
//     mu/done guard precedent

import (
	"context"
	"net"
	"net/http"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratelimitservicev3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// DecodeHeaders implements the §4.6 decode-side dispatch. The 4-step body is
// documented at the file-header. Returns Continue when no descriptors are
// produced OR when the chain-seed plumbing is missing (synthetic-stream test
// path); StopIteration when the async ShouldRateLimit goroutine is launched.
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	// Step 1 + 2: read accessors + build descriptors via the §4 engine.
	// The dcb nil-guard tolerates the test path where a *filter is constructed
	// without a callbacks reference (production HCM dispatch always wires dcb
	// at chain-build time via SetDecoderCallbacks).
	var (
		routeRLs    []*routev3.RateLimit
		vhostRLs    []*routev3.RateLimit
		remoteAddr  net.Addr
		clusterName string // empty at 24.1 — see file-header
	)
	if f.dcb != nil {
		routeRLs = f.dcb.RouteRateLimits()
		vhostRLs = f.dcb.VirtualHostRateLimits()
		remoteAddr = f.dcb.DownstreamRemoteAddr()
	}

	descriptors := buildDescriptors(routeRLs, vhostRLs, headers, remoteAddr, clusterName)

	// Step 3: zero-descriptor short-circuit — no RLS call (zero-regression path).
	if len(descriptors) == 0 {
		return envoyhttp.Continue
	}

	// Defensive: production wires rlsCallFn at New-factory time; a nil here
	// is a test-shape problem. Fail open (Continue) rather than blackholing
	// the request.
	if f.cc == nil || f.cc.rlsCallFn == nil {
		return envoyhttp.Continue
	}

	// Step 4: build the per-stream cancellable context (the ext_authz
	// extauthz.go:1037 precedent). Store callCancel on f so OnDestroy can
	// fire it; the captured callCtx is the goroutine's deadline anchor.
	// The per-call timeout (cc.timeout) is applied INSIDE the rlsCallFn
	// closure by *grpcclient.RateLimitClient.ShouldRateLimit per ADR-0158;
	// the cancellation chains via context.WithTimeout's AND-of-cancellation.
	callCtx, callCancel := context.WithCancel(context.Background())
	f.mu.Lock()
	f.callCtx = callCtx
	f.callCancel = callCancel
	f.mu.Unlock()

	// Build the RateLimitRequest envelope. Per AMEND-6 the descriptors are
	// emitted in policy order (the engine produces one descriptor per
	// surviving policy in input-slice order). hits_addend is NOT set (the
	// proto field is optional; 24.1 always charges 1 hit per request — the
	// proto-zero default — per parent SPEC §3.1).
	req := &ratelimitservicev3.RateLimitRequest{
		Domain:      f.cc.domain,
		Descriptors: descriptors,
	}

	// Launch the async goroutine. The OK / OVER_LIMIT / error disposition
	// dispatch lives in dispositions.go::applyDisposition; the resume guard
	// (mu / done) is acquired here (mirrors extauthz.go:1044).
	go func() {
		resp, err := f.cc.rlsCallFn(callCtx, req)

		f.mu.Lock()
		defer f.mu.Unlock()

		// If OnDestroy fired before us, the stream is gone — do NOT touch dcb.
		if f.done {
			return
		}
		f.applyDisposition(headers, resp, err)
	}()

	return envoyhttp.StopIteration
}
