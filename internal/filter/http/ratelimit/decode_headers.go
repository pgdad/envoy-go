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
	"strings"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratelimitfilterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	ratelimitservicev3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"

	"github.com/pgdad/envoy-go/internal/dynamicmetadata"
	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// extractRawQueryFromPath returns the query-string portion of the request's
// :path pseudo-header (without the leading '?'), or "" when the header is
// absent OR carries no query. The :path header is injected by HCM dispatch
// at H1 connection.go + H2 h2dispatch.go (Phase 16 Task 14). Used by the
// query_parameters + query_parameter_value_match descriptor actions per
// parent SPEC §4.1 rows 9, 10.
func extractRawQueryFromPath(headers http.Header) string {
	path := headers.Get(":path")
	if path == "" {
		return ""
	}
	if i := strings.IndexByte(path, '?'); i >= 0 {
		return path[i+1:]
	}
	return ""
}

// getNodeServiceCluster is a nil-safe accessor for cc.nodeServiceCluster. Test
// paths may construct a *filter without a compiledConfig (the synthetic-stream
// dispatch path); returns "" in that case.
func (c *compiledConfig) getNodeServiceCluster() string {
	if c == nil {
		return ""
	}
	return c.nodeServiceCluster
}

// DecodeHeaders implements the §4.6 decode-side dispatch. The 4-step body is
// documented at the file-header. Returns Continue when no descriptors are
// produced OR when the chain-seed plumbing is missing (synthetic-stream test
// path); StopIteration when the async ShouldRateLimit goroutine is launched.
//
// Phase-24.2 Task 4 (D-RL11 / parent SPEC §4.3 + AMEND-4 + AMEND-5):
//
//  1. Resolve the per-route `RateLimitPerRoute` via `f.dcb.RequestRouteConfig()`
//     (ADR-0073 most-specific override). Project to *compiledPerRoute via
//     `compilePerRouteForRequest`.
//  2. If the per-route `rate_limits[]` is non-empty (Axis-A): REPLACE the
//     route-table rate_limits with the per-route list AND zero the vhost-table
//     rate_limits. The walker emits descriptors ONLY for the per-route list
//     (AMEND-4 unconditional early-return; the route-table + vhost-table +
//     vh_rate_limits enum are bypassed entirely).
//  3. Otherwise (Axis-B): compute the vhost-walk disposition from the
//     `vh_rate_limits` enum + the legacy `include_vh_rate_limits` bool (AMEND-5
//     force-include trumps the enum). The walker honors the OVERRIDE / INCLUDE
//     / IGNORE table per §4.3.
//  4. Domain override (AMEND-4): a non-empty per-route `domain` overrides the
//     filter-config `domain` in the outbound RateLimitRequest.
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
		// Phase 24.2 Task 1 (D-RL8): the metadata-action source inputs +
		// the source_cluster / query_parameters value sources.
		dynBucket *dynamicmetadata.Bucket
		routeMD   *corev3.Metadata
		// Phase 24.2 Task 4 (D-RL11): the per-route RateLimitPerRoute
		// projection + the legacy `RouteAction.include_vh_rate_limits` bool.
		pr                  *compiledPerRoute
		legacyForceIncludeV bool
	)
	if f.dcb != nil {
		routeRLs = f.dcb.RouteRateLimits()
		vhostRLs = f.dcb.VirtualHostRateLimits()
		remoteAddr = f.dcb.DownstreamRemoteAddr()
		dynBucket = f.dcb.DynamicMetadata()
		routeMD = f.dcb.RouteMetadata()
		// Phase 24.2 Task 4 (D-RL11): resolve the per-route RateLimitPerRoute
		// via the ADR-0073 most-specific-override path. compilePerRouteForRequest
		// is nil-tolerant: nil msg + wrong-type both return nil (the engine's
		// "no per-route override at this tier" branch). RouteIncludeVhRateLimits
		// is the chain-seeded legacy bool (D-RL11 AMEND-5 force-include arm).
		pr = compilePerRouteForRequest(f.dcb.RequestRouteConfig())
		legacyForceIncludeV = f.dcb.RouteIncludeVhRateLimits()
	}

	// Phase 24.2 Task 1: extract the raw query string from the :path
	// pseudo-header for the query_parameters + query_parameter_value_match
	// descriptor actions (parent SPEC §4.1 rows 9, 10). The :path header is
	// injected by HCM dispatch at connection.go (H1) + h2dispatch.go (H2) per
	// the Phase 16 Task 14 plumbing pattern. Empty when the request has no
	// query string OR when the synthetic-stream test path bypasses HCM.
	rawQuery := extractRawQueryFromPath(headers)

	// Phase 24.2 Task 4 (D-RL11 / AMEND-4): Axis-A early-return + Axis-B
	// composition table. The two arms are computed BEFORE the engine walk to
	// honor the AMEND-4 unconditional precedence (Axis-A trumps Axis-B
	// entirely) AND the AMEND-5 legacy-bool-trumps-enum precedence within
	// Axis-B.
	walkRouteRLs := routeRLs
	walkVhostRLs := vhostRLs
	walkMode := vhostWalkOverrideDefault
	if pr != nil && len(pr.rateLimits) > 0 {
		// Axis-A early-return: substitute the per-route embedded list for
		// the route-table; zero out vhost. The §4.3 enum + the legacy bool
		// are bypassed entirely per AMEND-4. Stage filtering (§4.4) still
		// applies via the per-policy `Stage.Value == filterStage` check
		// inside the engine.
		walkRouteRLs = pr.rateLimits
		walkVhostRLs = nil
		// walkMode stays at vhostWalkOverrideDefault — irrelevant since
		// vhost is nil.
	} else {
		// Axis-B disposition: AMEND-5 legacy force-include trumps the enum.
		switch {
		case legacyForceIncludeV:
			walkMode = vhostWalkAlways
		case pr != nil:
			switch pr.vhRateLimits {
			case ratelimitfilterv3.RateLimitPerRoute_INCLUDE:
				walkMode = vhostWalkAlways
			case ratelimitfilterv3.RateLimitPerRoute_IGNORE:
				walkMode = vhostWalkNever
			case ratelimitfilterv3.RateLimitPerRoute_OVERRIDE:
				fallthrough
			default:
				walkMode = vhostWalkOverrideDefault
			}
		default:
			// No per-route override AND no legacy force-include — 24.1
			// OVERRIDE-default baseline (walk vhost only when route empty).
			walkMode = vhostWalkOverrideDefault
		}
	}

	descriptors := buildDescriptorsExt(descriptorInputs{
		routeRateLimits:    walkRouteRLs,
		vhostRateLimits:    walkVhostRLs,
		headers:            headers,
		remoteAddr:         remoteAddr,
		clusterName:        clusterName,
		nodeServiceCluster: f.cc.getNodeServiceCluster(),
		rawQuery:           rawQuery,
		dynamicMetadata:    dynBucket,
		routeMetadata:      routeMD,
		// Phase-24.2 Task 2 (§4.4): the filter's configured stage selects the
		// route/vhost policy bucket walked at request time. Default 0 honors
		// the upstream `getApplicableRateLimit(0)` proto-zero default; non-
		// zero filter stages were validated <= 10 at parse-time.
		filterStage: f.cc.getStage(),
		// Phase-24.2 Task 4 (§4.3): the computed Axis-B vhost-walk disposition.
		vhostWalkMode: walkMode,
	})

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
	f.callCancel = callCancel
	f.mu.Unlock()

	// Phase 24.2 Task 4 (D-RL11 / AMEND-4): per-route domain override. A
	// non-empty per-route `domain` wins over the filter-config `domain`;
	// empty (or nil pr) falls back to the filter-config value. The fallback
	// includes both "no per-route config" AND "per-route config with empty
	// domain" — matches upstream parity.
	domain := f.cc.domain
	if pr != nil && pr.domain != "" {
		domain = pr.domain
	}

	// Build the RateLimitRequest envelope. Per AMEND-6 the descriptors are
	// emitted in policy order (the engine produces one descriptor per
	// surviving policy in input-slice order). hits_addend is NOT set (the
	// proto field is optional; 24.1 always charges 1 hit per request — the
	// proto-zero default — per parent SPEC §3.1).
	req := &ratelimitservicev3.RateLimitRequest{
		Domain:      domain,
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
