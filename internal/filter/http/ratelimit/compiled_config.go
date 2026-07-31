package ratelimit

// compiled_config.go — the per-listener parsed + validated filter config per
// parent SPEC §6.1 + AMEND-3 (13-field roster) + the FULL §5 PARSE-REJECT
// roster + the EXPORTED ValidateRouteRateLimits validator (D-RL2) consumed by
// HCM at Task 5.
//
// # buildCompiledConfig — PARSE-REJECT discipline
//
// §5.1 RATIFIED-from-PGV / config-validation arms (upstream rejects too):
//
//  1. domain empty (PGV min_len:1)
//  2. rate_limit_service absent (PGV (message).required)
//  3. stage > 10 (PGV lte:10)
//  4. request_type not in {internal, external, both, ""} (PGV in:[...])
//  5. response_headers_to_add > 10 entries (PGV max_items:10)
//  6. rate_limit_service.grpc_service absent
//  7. rate_limit_service.grpc_service: google_grpc arm not supported (envoy-go-strict)
//  8. rate_limit_service.grpc_service: envoy_grpc arm required (target_specifier set)
//  9. rate_limit_service.grpc_service: envoy_grpc.cluster_name non-empty
//  10. cluster manager not available (FactoryCtx.ClusterManager nil)
//  11. cluster unknown
//  12. cluster not HTTP/2 (UseH2() false)
//
// Arms 6-12 mirror the ext_authz buildGRPCCheckFn cluster-load gates (REUSE 1)
// adapted to the "ratelimit:" wording prefix per ADR-0080 byte-stable error
// discipline.
//
// §5.2 envoy-go-strict project-local arms (route-level — exposed via the
// EXPORTED ValidateRouteRateLimits validator called by HCM at Task 5):
//
//  13. route/vhost RateLimit.disable_key non-empty (runtime keying deferred)
//  14. RateLimit_Action.extension arm set (descriptor-producer extension-point deferred)
//  15. RateLimit_Action.dynamic_metadata arm set (deprecated; use 'metadata')
//
// §5.1 charset arm added by phase 81 (stats-name-charset-guards):
//
//  16. stat_prefix carries a character invalid for a metric name (would panic
//      newFilterStats -> NewCounterIfAbsent -> checkName at listener build).
//      Fires AFTER arms 6-12 because the assembled-name probe needs the
//      resolved RLS cluster name — see the site comment in buildCompiledConfig.
//
// # AMEND-3 defaults / clamps
//
// After all PARSE-REJECT arms pass, the AMEND-3 defaults/clamps fire:
//   - timeout absent ⇒ 20ms
//   - request_type empty ⇒ "both"
//   - rate_limited_status absent OR < 400 ⇒ 429 (per the proto doc-comment +
//     toErrorCode mirror, ratelimit.h:140-146)
//   - status_on_error absent OR outside [100,511] ⇒ 500 (per toRatelimitServerErrorCode,
//     ratelimit.h:148-154)
//   - enable_x_ratelimit_headers parsed (DRAFT_VERSION_03 ⇒ true; OFF ⇒ false)
//     but NOT emitted at 24.1 per D-RL7 — the encode-side injection lands at 24.2
//
// # Cross-references
//
//   - ADR-0072 (boot-time-fail-fast)
//   - ADR-0080 (envoy-go-strict departure discipline + byte-stable error wording)
//   - ADR-0200 (route-level RTDS/runtime + descriptor-action deferral PARSE-REJECTs —
//     the §Decision + §Consequences bodies anchor at this commit per ADR-0044
//     in-place edit discipline)
//   - parent SPEC §5.1 + §5.2 + §6.1 (the PARSE-REJECT roster + the 13-field
//     compiledConfig shape)
//   - parent SPEC §1.1 AMEND-2 + AMEND-3 + AMEND-7 + AMEND-11 (empirical pins
//     codified by this Task)
//   - ext_authz precedent: internal/filter/http/extauthz/check.go::buildGRPCCheckFn
//     (the REUSE 1 cluster-load gate pattern adapted here)

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratelimitfilterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	ratelimitservicev3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
)

// rlsCallFn is the mode-agnostic outbound ShouldRateLimit closure captured at
// New-factory time per the ext_authz `checkFn` precedent (extauthz.go:54).
// Production wires a closure over *grpcclient.RateLimitClient; tests inject a
// synthetic fn to bypass the gRPC layer. The function receives a per-stream
// cancellable context (the ext_authz callCtx/callCancel pair at Task 7's
// dispatchOutboundCheck) so OnDestroy-driven cancellation propagates through.
type rlsCallFn func(ctx context.Context, req *ratelimitservicev3.RateLimitRequest) (*ratelimitservicev3.RateLimitResponse, error)

// ----------------------------------------------------------------------------
// Byte-stable PARSE-REJECT wording constants per ADR-0080 + parent SPEC §5.
// Each constant is a package-level `const` string (NOT a fmt.Errorf format
// string) so it can be asserted byte-exact by TestParseRejectConstants_ByteStable
// against the parent SPEC §5.1 + §5.2 reference roster. The 5.1 cluster-load
// constants are PREFIX-only — the runtime arm appends the cluster name (a %q-
// quoted suffix per fmt.Errorf, matching the ext_authz precedent at
// internal/filter/http/extauthz/check.go).
const (
	// ---- §5.1 RATIFIED-from-PGV / config-validation arms ----

	// parseRejectDomainRequired — empty domain per PGV min_len:1 (validate.go:61-70).
	parseRejectDomainRequired = "ratelimit: domain is required"

	// parseRejectRateLimitServiceRequired — rate_limit_service nil per PGV
	// (message).required (validate.go:127-136).
	parseRejectRateLimitServiceRequired = "ratelimit: rate_limit_service is required"

	// parseRejectStageTooHigh — stage > 10 per PGV lte:10 (validate.go:72-81).
	parseRejectStageTooHigh = "ratelimit: stage must be <= 10"

	// parseRejectRequestTypeInvalid — request_type not in {internal,external,both,""}
	// per PGV in (validate.go:362-367).
	parseRejectRequestTypeInvalid = "ratelimit: request_type must be one of internal|external|both"

	// parseRejectResponseHeadersTooMany — response_headers_to_add > 10 entries
	// per PGV max_items:10.
	parseRejectResponseHeadersTooMany = "ratelimit: response_headers_to_add accepts at most 10 items"

	// ---- §5.1 cluster-load arms (REUSE 1; ext_authz buildGRPCCheckFn precedent) ----

	// parseRejectGrpcServiceRequired — rate_limit_service.grpc_service nil.
	parseRejectGrpcServiceRequired = "ratelimit: rate_limit_service.grpc_service is required"

	// parseRejectGoogleGrpcNotSupported — the google_grpc native-channel arm is
	// envoy-go-strict-rejected; envoy-go uses google.golang.org/grpc directly.
	// Mirrors ext_authz wording at extauthz/check.go:538 with the "ratelimit:"
	// prefix substitution.
	parseRejectGoogleGrpcNotSupported = "ratelimit: rate_limit_service.grpc_service: google_grpc arm not supported (envoy-go uses google.golang.org/grpc directly)"

	// parseRejectEnvoyGrpcArmRequired — the GrpcService target_specifier oneof
	// is unset (no envoy_grpc, no google_grpc). Mirrors ext_authz/check.go:545.
	parseRejectEnvoyGrpcArmRequired = "ratelimit: rate_limit_service.grpc_service: envoy_grpc arm required (target_specifier must be set)"

	// parseRejectEnvoyGrpcClusterNameEmpty — envoy_grpc.cluster_name empty per
	// PGV min_len:1. Mirrors ext_authz/check.go:549.
	parseRejectEnvoyGrpcClusterNameEmpty = "ratelimit: rate_limit_service.grpc_service: envoy_grpc.cluster_name must be non-empty"

	// parseRejectClusterManagerNotAvailable — FactoryCtx.ClusterManager is nil
	// (test path with no cluster-bearing context per ADR-0085). Mirrors
	// ext_authz/check.go:555.
	parseRejectClusterManagerNotAvailable = "ratelimit: rate_limit_service.grpc_service: cluster manager not available (FactoryCtx.ClusterManager is nil)"

	// parseRejectUnknownClusterFmt + parseRejectClusterNotH2Fmt are runtime
	// fmt-error formats (cluster name appears in the wording per ext_authz
	// precedent). NOT asserted byte-stable as consts in
	// TestParseRejectConstants_ByteStable — covered by the §5.1 PARSE-REJECT
	// rows that match the fully-quoted runtime wording.

	// parseRejectStatPrefixInvalid — the filter-config `stat_prefix` (AMEND-3
	// 13th field) carries a character that internal/stats.IsValidName rejects.
	// Unguarded, such a token reaches newFilterStats -> NewCounterIfAbsent,
	// whose checkName PANICS the process at listener construction (registry.go
	// checkName / getOrRegister). ADR-0059 keeps the panic correct for
	// programmer error; this arm is the matching operator-input boundary check.
	// envoy-go-strict departure is NOT claimed here — upstream Envoy's
	// stat-name sanitizer accepts the same alphabet.
	parseRejectStatPrefixInvalid = "ratelimit: stat_prefix contains characters invalid for a metric name"

	// ---- §5.2 envoy-go-strict project-local arms (ADR-0200) ----

	// parseRejectRouteRateLimitDisableKey — route/vhost RateLimit.disable_key
	// non-empty triggers HCM-parse-time PARSE-REJECT (the route-level runtime
	// surface per AMEND-7). Forward-pointer to the future Runtime/RTDS family
	// phase. envoy-go-strict departure (upstream consults the runtime layer);
	// NOT a differential boot-reject candidate (upstream ACCEPTS this).
	parseRejectRouteRateLimitDisableKey = "ratelimit: rate_limits[].disable_key is not yet supported (runtime keying deferred)"

	// parseRejectRouteRateLimitActionExtension — the 'extension' descriptor
	// action (TypedExtensionConfig for category envoy.rate_limit_descriptors)
	// needs a descriptor-producer extension-point sub-framework; deferred per
	// EXTRACT-NOW-only-when-≥2 discipline. envoy-go-strict departure (upstream
	// accepts it); NOT a differential boot-reject candidate.
	parseRejectRouteRateLimitActionExtension = "ratelimit: the 'extension' descriptor action is not yet supported"

	// parseRejectRouteRateLimitActionDynamicMetadata — the deprecated
	// dynamic_metadata descriptor action (superseded in-proto by 'metadata').
	// envoy-go-strict-rejects the deprecated arm. envoy-go-strict departure
	// (upstream accepts it); NOT a differential boot-reject candidate.
	parseRejectRouteRateLimitActionDynamicMetadata = "ratelimit: the deprecated 'dynamic_metadata' descriptor action is not supported; use 'metadata'"
)

// ----------------------------------------------------------------------------
// Defaults + clamp boundaries per parent SPEC §6.1 + AMEND-3.
// ----------------------------------------------------------------------------

const (
	// defaultTimeout is the rate-limit-service RPC timeout when the proto
	// field is absent (per the proto doc + AMEND-3).
	defaultTimeout = 20 * time.Millisecond

	// defaultRateLimitedStatus is the OVER_LIMIT downstream response status
	// when rate_limited_status is absent (per the proto doc + AMEND-3).
	defaultRateLimitedStatus = 429

	// minRateLimitedStatus is the proto-stipulated lower bound for
	// rate_limited_status; anything < 400 clamps to defaultRateLimitedStatus.
	minRateLimitedStatus = 400

	// defaultStatusOnError is the error-path downstream response status
	// when status_on_error is absent (per the proto doc + AMEND-3).
	defaultStatusOnError = 500

	// minStatusOnError + maxStatusOnError are the proto-stipulated clamp
	// boundaries for status_on_error per ratelimit.h:148-154
	// toRatelimitServerErrorCode (HTTP status codes in [100, 511]).
	minStatusOnError = 100
	maxStatusOnError = 511

	// maxStage is the inclusive upper bound on the stage field per PGV lte:10.
	maxStage = 10

	// maxResponseHeadersToAdd is the inclusive upper bound on the
	// response_headers_to_add list length per PGV max_items:10.
	maxResponseHeadersToAdd = 10

	// defaultRequestType is the request_type honored when the proto field is
	// empty (per validate.go:362-367 + the proto doc).
	defaultRequestType = "both"
)

// ----------------------------------------------------------------------------
// compiledConfig — the post-parse per-listener filter config per parent
// SPEC §6.1 + AMEND-3 (the 13-field roster).
// ----------------------------------------------------------------------------

// headerKV is the post-parse representation of a *corev3.HeaderValueOption
// entry from response_headers_to_add. Canonical-cased header name; verbatim
// value; the proto-level append_action is NOT modeled at 24.1 — the encode-
// side X-RateLimit injection lands at 24.2 per D-RL7, and the OVER_LIMIT
// reply emission at Task 7's dispositions.go treats all filter-config
// response_headers as APPEND_IF_EXISTS_OR_ADD (the proto default; matches
// the ratelimit.cc:278-283 OVER_LIMIT-reply codepath).
type headerKV struct {
	name  string
	value string
}

// compiledConfig is the per-listener post-parse filter config per parent SPEC
// §6.1 + AMEND-3. Populated by buildCompiledConfig at HCM-build time per
// ADR-0072 boot-time-fail-fast. Read-only after construction.
//
// The 13-field AMEND-3 roster is preserved verbatim; the field order mirrors
// parent SPEC §6.1 for cross-referenceability. Each field is documented with
// the parent SPEC AMEND citation + the proto field name it derives from + the
// honored default/clamp policy.
type compiledConfig struct {
	// domain is the rate-limit domain string sent to the RLS in every
	// RateLimitRequest envelope. REQUIRED (non-empty per §5.1 arm 1).
	// Derived from proto field 1 (domain).
	domain string

	// stage selects the route/vhost RateLimit-policy stage bucket (range
	// 0..10 per §5.1 arm 3). At 24.1 the engine evaluated only the default
	// stage-0 bucket; phase-24.2 Task 2 generalizes — at request time the
	// walker filters by `p.GetStage().GetValue() == cc.stage` (descriptors.go
	// :336), whose observable semantics are equivalent to indexing
	// `bucketRateLimitsByStage(...)[cc.stage]` per §4.4 upstream
	// `getApplicableRateLimit(stage)`. The `bucketRateLimitsByStage` helper
	// itself is exposed for parse-time occupancy assertions + future
	// composition reuse (Task 3 per-route, Task 4 Axis-B). Accessed via the
	// nil-safe `cc.getStage()` accessor on the decode-headers path.
	// Derived from proto field 2 (stage).
	stage uint32

	// requestType is the internal/external classification filter. Empty proto
	// value ⇒ "both" (defaultRequestType). Honored values: "internal" /
	// "external" / "both" — anything else PARSE-REJECTs (§5.1 arm 4).
	// Derived from proto field 3 (request_type).
	//
	// At 24.1 the request_type FILTER is PARSED-NOT-CONSUMED: the
	// `x-envoy-internal` request-header gate that selects the request bucket
	// lands at a future phase. 24.1 walks ALL policies regardless of the
	// internal/external classification.
	//
	//nolint:unused // parsed at 24.1; x-envoy-internal gate deferred (forward-pointer)
	requestType string

	// timeout is the per-RPC deadline for ShouldRateLimit calls to the RLS.
	// Absent proto field ⇒ defaultTimeout (20ms per AMEND-3).
	// Derived from proto field 4 (timeout).
	timeout time.Duration

	// failureModeDeny is the error-disposition gate: false (the proto-zero
	// default) ⇒ fail-OPEN (allow on RLS error); true ⇒ fail-CLOSED (return
	// statusOnError on RLS error).
	// Derived from proto field 5 (failure_mode_deny).
	failureModeDeny bool

	// rateLimitedAsResourceExhausted controls the OVER_LIMIT gRPC code:
	// true ⇒ RESOURCE_EXHAUSTED (8) for gRPC-request OVER_LIMIT replies;
	// false ⇒ UNAVAILABLE (14). HTTP responses always carry rateLimitedStatus.
	// Derived from proto field 6 (rate_limited_as_resource_exhausted).
	//
	// At 24.1 the gRPC-trailer envelope on OVER_LIMIT replies for gRPC-shaped
	// downstream requests is NOT yet emitted (parent SPEC §4.7 + the
	// admission_control "ABSENT-by-API" precedent for rc-details — the same
	// applies to gRPC trailers). The field is consumed by
	// rateLimitedAsResourceExhaustedGrpcCode for forward 24.2 consumption.
	rateLimitedAsResourceExhausted bool

	// rlsClusterName is the upstream cluster carrying the RateLimitService
	// gRPC server. Derived from
	// proto.rate_limit_service.grpc_service.envoy_grpc.cluster_name.
	// REQUIRED non-empty + REQUIRED resolvable + REQUIRED HTTP/2 (§5.1 arms
	// 6-12 enforce). Captured here for Task 7 NewRateLimitClient construction
	// + the AMEND-1 / AMEND-10 cluster-scoped stat namespace anchor.
	rlsClusterName string

	// enableXRateLimitHeaders is the DRAFT_VERSION_03 X-RateLimit header
	// injection gate parsed at 24.1 per AMEND-3 but NOT emitted at 24.1 per
	// D-RL7 — the encode-side injection lands at 24.2 (encode.go + headers.go
	// per parent SPEC §6.6). True iff proto enum value ==
	// RateLimit_DRAFT_VERSION_03; false for RateLimit_OFF (the proto-zero
	// default). Derived from proto field 8 (enable_x_ratelimit_headers).
	//
	//nolint:unused // parsed at 24.1; consumed at 24.2 encode-side injection
	enableXRateLimitHeaders bool

	// disableXEnvoyRateLimitedHeader suppresses the `x-envoy-ratelimited: true`
	// header on OVER_LIMIT replies when true (otherwise the header is added
	// per ratelimit.cc OVER_LIMIT codepath; AMEND-8).
	// Derived from proto field 9 (disable_x_envoy_ratelimited_header).
	disableXEnvoyRateLimitedHeader bool

	// rateLimitedStatus is the OVER_LIMIT downstream HTTP response status.
	// Absent proto field ⇒ 429 (defaultRateLimitedStatus); proto value < 400
	// ⇒ 429 (clamp per the proto doc + toErrorCode mirror).
	// Derived from proto field 10 (rate_limited_status).
	rateLimitedStatus int

	// statusOnError is the error-path downstream HTTP response status when
	// failureModeDeny is true. Absent proto field OR value outside [100,511]
	// ⇒ 500 (defaultStatusOnError; clamp per toRatelimitServerErrorCode,
	// ratelimit.h:148-154).
	// Derived from proto field 12 (status_on_error).
	statusOnError int

	// statPrefix is the AMEND-1 stat-namespace modulator: when non-empty the
	// cluster-scoped 4-counter surface registers under
	// `cluster.<rls>.ratelimit.<statPrefix>.<stat>` (vs the default
	// `cluster.<rls>.ratelimit.<stat>`). Empty ⇒ no per-filter modulator.
	// Derived from proto field 13 (stat_prefix).
	statPrefix string

	// nodeServiceCluster is the Envoy NODE's `service-cluster` name as
	// captured at New-factory time from FactoryCtx.NodeServiceCluster. Phase
	// 24.2 Task 1 first-use anchor: consumed by the `source_cluster`
	// descriptor action per parent SPEC §4.1 row 1 (upstream
	// router_ratelimit.cc:89-90 always-true semantics). Empty string is the
	// documented nil-passthrough — the action then emits an empty-value
	// descriptor entry (descriptor survives; the key "source_cluster" is
	// non-empty so the §4.5 push_back fires).
	nodeServiceCluster string

	// responseHeadersToAdd is the pre-compiled list of filter-config headers
	// appended to OVER_LIMIT replies (per ratelimit.cc:278-283 + AMEND-8).
	// Bounded by maxResponseHeadersToAdd (10) per §5.1 arm 5. Compiled here
	// from proto field 11 (response_headers_to_add); the proto's
	// HeaderValueOption.append_action is NOT modeled at 24.1 (treated as
	// APPEND_IF_EXISTS_OR_ADD — the proto-zero default; matches the
	// OVER_LIMIT-reply codepath which does NOT distinguish append modes).
	responseHeadersToAdd []headerKV

	// rlsCallFn is the outbound ShouldRateLimit closure captured by the
	// Task-7 `New` factory: wraps the *grpcclient.RateLimitClient + the
	// per-call timeout discipline. Tests inject a synthetic fn to bypass the
	// gRPC layer (the ext_authz `checkFn` precedent). Nil only in test-shaped
	// compiledConfigs that do NOT exercise the descriptor-dispatch path.
	rlsCallFn rlsCallFn

	// stats is the SHARED cluster-scoped 4-counter surface allocated by
	// `newFilterStats` at New-factory time per AMEND-1 + AMEND-10. May carry
	// nil counter fields when ctx.Stats was nil (per ADR-0085 nil-tolerance);
	// the dispositions Inc-paths nil-guard each counter. Read-only after
	// construction.
	stats *filterStats
}

// ----------------------------------------------------------------------------
// buildCompiledConfig — the proto-parser body per parent SPEC §6.1 + the FULL
// §5.1 PARSE-REJECT roster.
// ----------------------------------------------------------------------------

// buildCompiledConfig parses + validates a *ratelimitfilterv3.RateLimit
// envelope (wrapped in *anypb.Any) into a *compiledConfig per parent SPEC
// §6.1 + AMEND-3.
//
// Arm-firing order (mirrors the parent SPEC §5.1 table top-to-bottom; cluster-
// load gates fire LAST so the simpler config-shape PARSE-REJECTs surface
// before the cluster-manager-bearing arms):
//
//  1. typed_config nil ⇒ PARSE-REJECT (defensive; Task 7's New also guards)
//  2. typed_config unmarshal failure ⇒ wrapped-error PARSE-REJECT
//  3. domain empty ⇒ §5.1 arm 1
//  4. stage > 10 ⇒ §5.1 arm 3
//  5. request_type not in {"","internal","external","both"} ⇒ §5.1 arm 4
//  6. response_headers_to_add > 10 ⇒ §5.1 arm 5
//  7. rate_limit_service nil ⇒ §5.1 arm 2
//  8. rate_limit_service.grpc_service nil ⇒ §5.1 arm 6
//  9. google_grpc arm set ⇒ §5.1 arm 7 (envoy-go-strict)
//  10. envoy_grpc arm unset ⇒ §5.1 arm 8
//  11. envoy_grpc.cluster_name empty ⇒ §5.1 arm 9
//  12. ctx.ClusterManager nil ⇒ §5.1 arm 10
//  13. cluster unknown ⇒ §5.1 arm 11
//  14. cluster not HTTP/2 ⇒ §5.1 arm 12
//  15. apply AMEND-3 defaults/clamps + populate the 13-field cc + return
//
// Per ADR-0072 boot-time-fail-fast: any validation failure returns the byte-
// stable error string verbatim (the cluster-name-bearing arms 11+12 use
// fmt.Errorf with a %q-quoted suffix per the ext_authz precedent at
// extauthz/check.go:559+562). The outer typed_config unmarshal failure uses a
// wrapped error (the proto-decoder context is useful for the operator's first-
// pass diagnostic — matches the phase-23 admission_control precedent at
// compiled_config.go:256).
//
// The cluster-load arms (10-12) require ctx.ClusterManager to be non-nil; per
// ADR-0085 nil-tolerance, ctx.ClusterManager may be nil in test code that
// exercises the upstream-of-cluster-lookup arms — arm 10 surfaces a stable
// PARSE-REJECT in that case rather than panicking.
func buildCompiledConfig(typedConfig *anypb.Any, ctx envoyhttp.FactoryCtx) (*compiledConfig, error) {
	if typedConfig == nil {
		return nil, errors.New("ratelimit: typed_config required")
	}

	var raw ratelimitfilterv3.RateLimit
	if err := typedConfig.UnmarshalTo(&raw); err != nil {
		return nil, fmt.Errorf("ratelimit: typed_config unmarshal: %w", err)
	}

	// §5.1 Arm 1: domain non-empty.
	if raw.GetDomain() == "" {
		return nil, errors.New(parseRejectDomainRequired)
	}

	// §5.1 Arm 3: stage <= 10.
	if raw.GetStage() > maxStage {
		return nil, errors.New(parseRejectStageTooHigh)
	}

	// §5.1 Arm 4: request_type in {"", "internal", "external", "both"}.
	switch raw.GetRequestType() {
	case "", "internal", "external", "both":
		// allowed
	default:
		return nil, errors.New(parseRejectRequestTypeInvalid)
	}

	// §5.1 Arm 5: response_headers_to_add length <= 10.
	if len(raw.GetResponseHeadersToAdd()) > maxResponseHeadersToAdd {
		return nil, errors.New(parseRejectResponseHeadersTooMany)
	}

	// §5.1 Arm 2: rate_limit_service present.
	rls := raw.GetRateLimitService()
	if rls == nil {
		return nil, errors.New(parseRejectRateLimitServiceRequired)
	}

	// §5.1 Arms 6-12: the cluster-load gates (REUSE 1; ext_authz precedent).
	gs := rls.GetGrpcService()
	clusterName, err := validateGrpcServiceAndResolveCluster(gs, ctx)
	if err != nil {
		return nil, err
	}

	// §5.1 Arm 16 (phase 81 stats-name-charset-guards): stat_prefix must be
	// metric-name safe. Numbered 16 because §5.2 already owns 13-15.
	//
	// ORDERING IS DELIBERATE, NOT INCIDENTAL — DO NOT HOIST THIS ARM.
	// The probe below assembles the FULL counter name, which needs
	// `clusterName`; that value only exists after
	// validateGrpcServiceAndResolveCluster (arms 6-12) has run. Consequence:
	// a config carrying BOTH an unknown RLS cluster AND an invalid
	// stat_prefix reports the CLUSTER error, not the charset error. That
	// precedence is accepted — `clusterName` is pre-guaranteed metric-name
	// valid by arms 9/11 (it named a cluster the manager already loaded), so
	// the assembled probe isolates `sp` as the only untrusted segment. Moving
	// this arm above arms 6-12 would force a BARE-token probe, and
	// stats.IsValidName is whole-string anchored: at this INTERIOR segment
	// position a bare probe disagrees with the assembled one and would
	// boot-reject configs that today boot, serve and register a valid
	// counter. Pinned by TestBuildCompiledConfig_StatPrefixCharset's
	// ClusterErrorPrecedesCharsetError arm.
	//
	// The probe suffix must track newFilterStats (stats.go): the assembled
	// name is `cluster.<clusterName>.ratelimit.<stat_prefix>.<leaf>`.
	// `tok != ""` is kept because the empty case is LIVE and landed —
	// newFilterStats elides the segment wholesale (including its leading dot)
	// when stat_prefix is empty, so for an empty token this probe would not
	// be the name actually registered.
	if sp := raw.GetStatPrefix(); sp != "" {
		probe := "cluster." + clusterName + ".ratelimit." + sp + "." + statNameFailureModeAllowed
		if !stats.IsValidName(probe) {
			return nil, errors.New(parseRejectStatPrefixInvalid)
		}
	}

	// All PARSE-REJECT arms passed — populate the 13-field cc per AMEND-3.
	cc := &compiledConfig{
		domain:                         raw.GetDomain(),
		stage:                          raw.GetStage(),
		requestType:                    normalizeRequestType(raw.GetRequestType()),
		timeout:                        timeoutOrDefault(raw.GetTimeout()),
		failureModeDeny:                raw.GetFailureModeDeny(),
		rateLimitedAsResourceExhausted: raw.GetRateLimitedAsResourceExhausted(),
		rlsClusterName:                 clusterName,
		enableXRateLimitHeaders:        raw.GetEnableXRatelimitHeaders() == ratelimitfilterv3.RateLimit_DRAFT_VERSION_03,
		disableXEnvoyRateLimitedHeader: raw.GetDisableXEnvoyRatelimitedHeader(),
		rateLimitedStatus:              rateLimitedStatusOrClamp(raw.GetRateLimitedStatus()),
		statusOnError:                  statusOnErrorOrClamp(raw.GetStatusOnError()),
		statPrefix:                     raw.GetStatPrefix(),
		nodeServiceCluster:             ctx.NodeServiceCluster,
		responseHeadersToAdd:           compileResponseHeaders(raw.GetResponseHeadersToAdd()),
	}

	return cc, nil
}

// ----------------------------------------------------------------------------
// validateGrpcServiceAndResolveCluster — REUSE 1 (ext_authz buildGRPCCheckFn
// cluster-load gate pattern adapted to ratelimit wording).
// ----------------------------------------------------------------------------

// validateGrpcServiceAndResolveCluster runs the §5.1 arms 6-12 against
// rate_limit_service.grpc_service:
//
//  1. grpc_service non-nil
//  2. NOT the google_grpc arm (envoy-go-strict)
//  3. envoy_grpc target_specifier set
//  4. envoy_grpc.cluster_name non-empty
//  5. ctx.ClusterManager non-nil
//  6. cluster exists in the manager
//  7. cluster uses HTTP/2 (UseH2() returns true)
//
// Returns the resolved cluster name on success (captured into
// compiledConfig.rlsClusterName for Task 7 NewRateLimitClient).
//
// Adapted from internal/filter/http/extauthz/check.go::buildGRPCCheckFn arms
// 1-3 (the cluster-load portion; the AuthClient construction is deferred to
// Task 7's New body in this filter).
func validateGrpcServiceAndResolveCluster(gs *corev3.GrpcService, ctx envoyhttp.FactoryCtx) (string, error) {
	if gs == nil {
		return "", errors.New(parseRejectGrpcServiceRequired)
	}

	// §5.1 Arm 7: google_grpc arm not supported (envoy-go-strict).
	if _, isGoogle := gs.GetTargetSpecifier().(*corev3.GrpcService_GoogleGrpc_); isGoogle {
		return "", errors.New(parseRejectGoogleGrpcNotSupported)
	}

	// §5.1 Arm 8: envoy_grpc target_specifier required.
	envoyGrpc := gs.GetEnvoyGrpc()
	if envoyGrpc == nil {
		return "", errors.New(parseRejectEnvoyGrpcArmRequired)
	}

	// §5.1 Arm 9: envoy_grpc.cluster_name non-empty (PGV min_len:1 mirror).
	clusterName := envoyGrpc.GetClusterName()
	if clusterName == "" {
		return "", errors.New(parseRejectEnvoyGrpcClusterNameEmpty)
	}

	// §5.1 Arm 10: cluster manager available (FactoryCtx.ClusterManager
	// non-nil per ADR-0085 nil-tolerance).
	if ctx.ClusterManager == nil {
		return "", errors.New(parseRejectClusterManagerNotAvailable)
	}

	// §5.1 Arm 11: cluster exists.
	clu, ok := ctx.ClusterManager.Get(clusterName)
	if !ok {
		return "", fmt.Errorf("ratelimit: rate_limit_service.grpc_service: unknown cluster %q", clusterName)
	}

	// §5.1 Arm 12: cluster uses HTTP/2 (gRPC requires HTTP/2 framing).
	if !clu.UseH2() {
		return "", fmt.Errorf("ratelimit: rate_limit_service.grpc_service: cluster %q must have http2_protocol_options{} set (gRPC requires HTTP/2)", clusterName)
	}

	return clusterName, nil
}

// ----------------------------------------------------------------------------
// AMEND-3 defaults/clamps — small helpers kept adjacent to buildCompiledConfig.
// ----------------------------------------------------------------------------

// normalizeRequestType applies the empty-string ⇒ "both" default per AMEND-3.
// All non-empty inputs flow through validated by the §5.1 arm 4 PARSE-REJECT
// switch above.
func normalizeRequestType(rt string) string {
	if rt == "" {
		return defaultRequestType
	}
	return rt
}

// timeoutOrDefault applies the absent-proto-field ⇒ 20ms default per AMEND-3.
// A zero or negative duration is honored as default (matches the proto doc:
// "If not set, this defaults to 20ms" — proto-zero ≡ absent here).
func timeoutOrDefault(d *durationpb.Duration) time.Duration {
	if d == nil {
		return defaultTimeout
	}
	v := d.AsDuration()
	if v <= 0 {
		return defaultTimeout
	}
	return v
}

// rateLimitedStatusOrClamp applies the absent ⇒ 429 default + the <400 ⇒ 429
// clamp per the proto doc ("If this is set to < 400, 429 will be used
// instead.") + AMEND-3. Nil-tolerant (absent proto field).
func rateLimitedStatusOrClamp(hs *typev3.HttpStatus) int {
	if hs == nil {
		return defaultRateLimitedStatus
	}
	code := int(hs.GetCode())
	if code < minRateLimitedStatus {
		return defaultRateLimitedStatus
	}
	return code
}

// statusOnErrorOrClamp applies the absent ⇒ 500 default + the outside-
// [100,511] ⇒ 500 clamp per toRatelimitServerErrorCode (ratelimit.h:148-154)
// + AMEND-3. Nil-tolerant (absent proto field).
func statusOnErrorOrClamp(hs *typev3.HttpStatus) int {
	if hs == nil {
		return defaultStatusOnError
	}
	code := int(hs.GetCode())
	if code < minStatusOnError || code > maxStatusOnError {
		return defaultStatusOnError
	}
	return code
}

// compileResponseHeaders compiles the proto response_headers_to_add list into
// the post-parse []headerKV slice. Canonical-cases the header name; copies
// the value verbatim. An entry with an empty key is skipped (defensive — the
// proto's HeaderValue.key is PGV-required but the wrapper field is not, so
// guard against the partial-population corner). The bounds-check (§5.1 arm 5)
// already fired in buildCompiledConfig — this helper trusts its input length
// is in range.
func compileResponseHeaders(opts []*corev3.HeaderValueOption) []headerKV {
	if len(opts) == 0 {
		return nil
	}
	out := make([]headerKV, 0, len(opts))
	for _, hvo := range opts {
		hv := hvo.GetHeader()
		if hv == nil || hv.GetKey() == "" {
			continue
		}
		out = append(out, headerKV{
			name:  http.CanonicalHeaderKey(hv.GetKey()),
			value: hv.GetValue(),
		})
	}
	return out
}

// ----------------------------------------------------------------------------
// ValidateRouteRateLimits — EXPORTED §5.2 validator (D-RL2) consumed by HCM
// at Task 5 against the route-level + virtual-host-level
// []*routev3.RateLimit slices PARSED from the route_config envelope.
// ----------------------------------------------------------------------------

// ValidateRouteRateLimits runs the §5.2 envoy-go-strict project-local
// PARSE-REJECT arms against a slice of *routev3.RateLimit (the
// RouteAction.rate_limits or VirtualHost.rate_limits field). HCM calls this
// at boot-time per ADR-0072 + D-RL2 per parent SPEC §5.2 + ADR-0200 — the
// VALIDATION lives in the ratelimit package because the byte-stable wording
// + the §5.2 anti-roster (disable_key / extension / dynamic_metadata) is a
// ratelimit-filter concern; HCM is the call-site (Task 5 wires DELTA-2).
//
// Arms (per parent SPEC §5.2 + ADR-0200):
//
//  1. RateLimit.disable_key non-empty (runtime keying deferred — forward-pointer
//     to the future Runtime/RTDS family phase per AMEND-7)
//  2. RateLimit_Action.extension arm set (descriptor-producer extension-point
//     deferred per EXTRACT-NOW-only-when-≥2 discipline per AMEND-11)
//  3. RateLimit_Action.dynamic_metadata arm set (deprecated in-proto; use
//     'metadata' per AMEND-11)
//
// Also enforces the §5.1 Arm 3 PGV `lte:10` mirror against the per-POLICY
// stage (phase-24.2 Task 2). The filter-envelope `stage > 10` arm already
// fires at `buildCompiledConfig`; here the SAME byte-stable wording fires
// against any route/vhost policy whose `stage.value > 10`. Mirrors upstream
// PGV `lte:10` on `config.route.v3.RateLimit.stage` (route_components.pb.go:3275).
//
// Returns the FIRST violation encountered (slice-order then action-order
// within each *RateLimit). nil input + empty input both return nil.
//
// Per ADR-0080 byte-stable wording: each arm returns a package-level const
// string verbatim — asserted by TestParseRejectConstants_ByteStable.
//
// Consumer: Task 5's HCM route-table parse (per DELTA-2 / ADR-0198) calls
// this on the matched route's + virtual-host's rate_limits slice when
// constructing the per-stream FilterChain accessors. A non-nil return
// surfaces at HCM-build time per ADR-0072 boot-time-fail-fast.
func ValidateRouteRateLimits(rls []*routev3.RateLimit) error {
	for _, rl := range rls {
		if rl == nil {
			continue
		}
		// §5.2 Arm 1: disable_key non-empty (route-level runtime surface
		// deferred — the AMEND-7 envoy-go-strict departure).
		if rl.GetDisableKey() != "" {
			return errors.New(parseRejectRouteRateLimitDisableKey)
		}
		// Phase-24.2 Task 2: per-policy stage > 10 PARSE-REJECT (PGV `lte:10`
		// mirror; parent §4.4 + §5.1 Arm 3). `rl.GetStage()` is a
		// `*wrapperspb.UInt32Value`; .GetValue() is nil-safe (returns 0).
		if rl.GetStage().GetValue() > maxStage {
			return errors.New(parseRejectStageTooHigh)
		}
		for _, action := range rl.GetActions() {
			if action == nil {
				continue
			}
			switch action.GetActionSpecifier().(type) {
			case *routev3.RateLimit_Action_Extension:
				// §5.2 Arm 2: extension descriptor action deferred.
				return errors.New(parseRejectRouteRateLimitActionExtension)
			case *routev3.RateLimit_Action_DynamicMetadata:
				// §5.2 Arm 3: deprecated dynamic_metadata action rejected.
				return errors.New(parseRejectRouteRateLimitActionDynamicMetadata)
			}
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// bucketRateLimitsByStage — the §4.4 parse-time stage bucketing helper.
// ----------------------------------------------------------------------------

// bucketRateLimitsByStage partitions a slice of *routev3.RateLimit into
// `maxStage+1 = 11` slots indexed by `policy.GetStage().GetValue()` (nil
// UInt32Value ⇒ 0; the upstream proto default per PGV `lte:10` invariant on
// `config.route.v3.RateLimit.stage`).
//
// Mirrors the upstream `RateLimitConfigImpl::references[stage]` table sized
// `MAX_STAGE_NUMBER + 1 = 11` (rl.cc:539-550 + parent SPEC §4.4). Out-of-
// range policies (stage > maxStage) are SKIPPED here defensively;
// `ValidateRouteRateLimits` rejects them at HCM-parse-time per the byte-
// stable `parseRejectStageTooHigh` arm — this helper trusts the input has
// passed validation but does not panic on stragglers.
//
// At request time, the engine consults `buckets[filterStage]` per the
// `getApplicableRateLimit(stage)` upstream semantics — see
// `buildDescriptorsExt` in descriptors.go for the per-request selection.
//
// nil + empty inputs return an all-empty 11-slot array (each slot nil/empty);
// the engine then walks ZERO policies at the selected bucket which the §4.6
// zero-descriptor-short-circuit handles uniformly.
func bucketRateLimitsByStage(rls []*routev3.RateLimit) [maxStage + 1][]*routev3.RateLimit {
	var buckets [maxStage + 1][]*routev3.RateLimit
	for _, rl := range rls {
		if rl == nil {
			continue
		}
		// rl.GetStage() is a *wrapperspb.UInt32Value; nil ⇒ stage 0 (proto default).
		// .GetValue() is nil-safe.
		stage := rl.GetStage().GetValue()
		if stage > maxStage {
			// Out-of-range — would have been PARSE-REJECTed by
			// `ValidateRouteRateLimits`; defensive skip here.
			continue
		}
		buckets[stage] = append(buckets[stage], rl)
	}
	return buckets
}

// getStage is a nil-safe accessor for cc.stage. Test paths may construct a
// *filter without a compiledConfig (the synthetic-stream dispatch path);
// returns 0 (the default stage) in that case. The 0 fallback aligns with the
// proto-zero default — same bucket the 24.1 baseline walked.
func (c *compiledConfig) getStage() uint32 {
	if c == nil {
		return 0
	}
	return c.stage
}
