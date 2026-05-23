package ratelimit

// descriptors.go — the §4 descriptor-action engine per parent SPEC §4.1
// (per-action key/value/drop rules) + §4.5 (empty-action-drop discipline) +
// §4.3 (Axis-A early-return + OVERRIDE-default vhost walk) at the 24.1 CORE
// slice (5 actions). The remaining 5 actions + `stage` multi-stage bucketing
// + the Axis-B `vh_rate_limits` table land at 24.2.
//
// # The 5 CORE actions (AMEND-11 key defaults)
//
//   - generic_key            — entry key default "generic_key" (proto field
//     descriptor_key may override); entry value = descriptor_value. Drops the
//     WHOLE descriptor if descriptor_value is empty (router_ratelimit.cc:163,
//     166-183: returns false when value empty and no default_value).
//   - request_headers        — entry key REQUIRED from proto descriptor_key
//     (no default); entry value = first lookup of header_name from the
//     request. Header absent + skip_if_absent=false ⇒ drop the WHOLE
//     descriptor (action returns false); header absent + skip_if_absent=true
//     ⇒ skip this entry only, descriptor survives.
//   - remote_address         — entry key fixed "remote_address"; entry value
//     = downstream remote IP as a string (no port). Drops the WHOLE descriptor
//     if the downstream address is nil OR not an IP (extracted via the
//     extauthz addressFromNetAddr precedent: *net.TCPAddr.IP.String()).
//   - destination_cluster    — entry key fixed "destination_cluster"; entry
//     value = the routed cluster's name (passed in by the caller — the engine
//     is pure and does not look this up). Drops the WHOLE descriptor if the
//     cluster name is empty (rare; the matched routeEntry always has a name).
//   - header_value_match     — entry key default "header_match" (proto field
//     descriptor_key may override); entry value = descriptor_value (required).
//     The action evaluates the headers[] matchers as a vacuous-true AND-fold:
//     matched? = ALL matchers match (empty list ⇒ true). The entry is emitted
//     iff matched? == expect_match (default true); otherwise the WHOLE
//     descriptor is dropped.
//
// # §4.5 empty-action-drop discipline (TWO behaviors)
//
//   1. Action returns false (the "drop" outcome) ⇒ the WHOLE descriptor is
//      discarded AND the per-policy actions loop breaks immediately.
//      Per router_ratelimit.cc:21-39 + ratelimit.cc:483-485.
//   2. Action returns true but produces an EMPTY key/value pair (the
//      "skip-entry" outcome) ⇒ that single entry is skipped; the descriptor
//      survives and the loop continues to the next action.
//      Per router_ratelimit.cc:34-36 (`if (!key.empty()) push_back`).
//
// # §4.3 cross-tier composition at 24.1 (D-RL6)
//
// 24.1 implements ONLY the OVERRIDE-default arm of the §4.3 cross-tier
// composition table (D-RL6). The full Axis-B `vh_rate_limits` table
// (INCLUDE / IGNORE / `include_vh_rate_limits` legacy override) + the
// `RateLimitPerRoute` Axis-A embedded policy + the per-route `domain`
// override land at 24.2.
//
// At 24.1 the walk order is the elegant simple form:
//
//   if route policies non-empty:
//     walk route policies → descriptors  (Axis-A early-return + OVERRIDE)
//   else:
//     walk vhost policies → descriptors  (OVERRIDE-default vhost walk)
//
// # Stage filtering (parent §4.4)
//
// Phase-24.2 Task 2 implements the FULL §4.4 multi-stage bucketing path: the
// engine walks ONLY the policies whose `RateLimit.Stage.Value` equals the
// filter's configured `stage` (compiledConfig.stage, parsed at 24.1; default
// 0). Equivalent to the upstream `getApplicableRateLimit(stage)` table-lookup
// (rl.cc:539-550 + parent SPEC §4.4 — `MAX_STAGE_NUMBER+1 = 11` invariant).
// The 24.1 stage-0-only baseline is preserved at `filterStage == 0`.
//
// PARSE-time PARSE-REJECTs `stage > 10` at BOTH levels:
//   - filter envelope: `buildCompiledConfig` arm (§5.1 Arm 3)
//   - per-policy: `ValidateRouteRateLimits` arm (added at phase-24.2 Task 2 —
//     PGV `lte:10` mirror on `config.route.v3.RateLimit.stage`)
//
// At parse time, `bucketRateLimitsByStage` (compiled_config.go) partitions a
// slice into `[maxStage+1]` bucket slots; the engine consumes the buckets
// implicitly via the per-request `filterStage == p.Stage.Value` check (the
// observable semantics are equivalent to a `buckets[filterStage]` selection).
//
// # AMEND-6 entries-in-action-list-order
//
// Within a single descriptor, `entries[i]` is built in `actions[i]` order
// (the per-action `populateDescriptor` calls append to a shared per-policy
// descriptor buffer in upstream order). The unit tests pin this property.
//
// # Cross-references
//
//   - parent SPEC §4 (descriptor-action engine; line-cited against
//     upstream source/common/router/router_ratelimit.cc + source/extensions/
//     filters/http/ratelimit/ratelimit.cc at v1.37.2)
//   - parent SPEC §4.1 (per-action key/value/drop rules — the 10-row table)
//   - parent SPEC §4.3 (cross-tier composition Axis-A + Axis-B)
//   - parent SPEC §4.5 (empty-action-drop TWO behaviors)
//   - parent SPEC §1.1 AMEND-6 (proto-number-faithful + entries action-order)
//   - parent SPEC §1.1 AMEND-11 (per-action key defaults)
//   - phase-24.1 PLAN D-RL6 (24.1 descriptor source = route/vhost rate_limits;
//     OVERRIDE-default vhost walk only; multi-stage + Axis-B at 24.2)
//   - phase-24.1 PLAN D-RL7 (X-RateLimit stub at 24.1; emission at 24.2)

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/ratelimit/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	metadatav3 "github.com/envoyproxy/go-control-plane/envoy/type/metadata/v3"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
)

// ----------------------------------------------------------------------------
// AMEND-11 key-default constants — exported as package-internal constants so
// the per-action helpers reference one name (no string-literal duplication).
// ----------------------------------------------------------------------------

const (
	// descriptorKeyGenericKeyDefault — default descriptor entry key for the
	// generic_key action when proto descriptor_key is empty (AMEND-11; mirrors
	// upstream router_ratelimit.cc:163 literal "generic_key").
	descriptorKeyGenericKeyDefault = "generic_key"

	// descriptorKeyHeaderValueMatchDefault — default descriptor entry key for
	// the header_value_match action when proto descriptor_key is empty
	// (AMEND-11; mirrors upstream router_ratelimit.cc:261 literal "header_match").
	descriptorKeyHeaderValueMatchDefault = "header_match"

	// descriptorKeyRemoteAddress — fixed entry key for the remote_address
	// action (no proto override; AMEND-11; mirrors upstream router_ratelimit.cc:126
	// literal "remote_address").
	descriptorKeyRemoteAddress = "remote_address"

	// descriptorKeyDestinationCluster — fixed entry key for the
	// destination_cluster action (no proto override; AMEND-11; mirrors upstream
	// router_ratelimit.cc:96 literal "destination_cluster").
	descriptorKeyDestinationCluster = "destination_cluster"

	// descriptorKeySourceCluster — fixed entry key for the source_cluster
	// action (no proto override; AMEND-11; mirrors upstream router_ratelimit.cc:89
	// literal "source_cluster").
	descriptorKeySourceCluster = "source_cluster"

	// descriptorKeyMaskedRemoteAddress — fixed entry key for the
	// masked_remote_address action (no proto override; AMEND-11; mirrors
	// upstream router_ratelimit.cc:141 literal "masked_remote_address").
	descriptorKeyMaskedRemoteAddress = "masked_remote_address"

	// descriptorKeyQueryParametersDefault — default entry key for the
	// query_parameters action when proto descriptor_key is empty (AMEND-11
	// SINGULAR per parent SPEC §1.1 AMEND-11; mirrors upstream literal
	// "query_param" — easy to typo as "query_params"; pinned byte-stable here).
	descriptorKeyQueryParametersDefault = "query_param"

	// descriptorKeyQueryParameterValueMatchDefault — default entry key for the
	// query_parameter_value_match action when proto descriptor_key is empty
	// (AMEND-11; mirrors upstream router_ratelimit.cc:262 literal "query_match").
	descriptorKeyQueryParameterValueMatchDefault = "query_match"
)

// ----------------------------------------------------------------------------
// AMEND-11 per-action defaults — defaults for the v4/v6 prefix-mask lengths
// when the proto wrappers are absent. Honor upstream rl.cc:141, 154-156
// (default 32 / 128).
// ----------------------------------------------------------------------------

const (
	// defaultV4PrefixMaskLen — default mask length for IPv4 masked_remote_address
	// per the proto doc ("Defaults to 32 when unset.").
	defaultV4PrefixMaskLen uint32 = 32

	// defaultV6PrefixMaskLen — default mask length for IPv6 masked_remote_address
	// per the proto doc ("Defaults to 128 when unset.").
	defaultV6PrefixMaskLen uint32 = 128
)

// ----------------------------------------------------------------------------
// vhostWalkMode — the §4.3 Axis-B vhost-walk disposition discriminator
// (phase-24.2 Task 4; parent SPEC §4.3 + AMEND-5).
//
// Three states matching the §4.3 decision table:
//
//   - vhostWalkOverrideDefault (zero-value; 24.1 baseline): walk vhost ONLY
//     when route is empty. Matches `RateLimitPerRoute.vh_rate_limits=OVERRIDE`
//     (the proto-zero default) AND the absence-of-per-route default. This is
//     the §4.3 table's OVERRIDE arm: VH walked only when route is empty
//     (yes ⇒ NO; no ⇒ YES).
//   - vhostWalkAlways: walk vhost UNCONDITIONALLY (alongside route). Matches
//     `vh_rate_limits=INCLUDE` AND the legacy
//     `RouteAction.include_vh_rate_limits=true` force-include override
//     (AMEND-5; the legacy bool trumps the enum when true).
//   - vhostWalkNever: SKIP vhost entirely (even when route is empty). Matches
//     `vh_rate_limits=IGNORE`.
//
// The disposition is computed by decode_headers.go from the per-route enum +
// the legacy bool BEFORE invoking buildDescriptorsExt; the engine just honors
// whatever the caller decides. Zero-value preserves the 24.1 baseline (the
// `buildDescriptors` 5-arg wrapper passes the zero value transparently).
// ----------------------------------------------------------------------------

type vhostWalkMode int

const (
	// vhostWalkOverrideDefault — walk vhost ONLY when route is empty (the
	// §4.3 OVERRIDE arm + 24.1 baseline; zero-value).
	vhostWalkOverrideDefault vhostWalkMode = iota
	// vhostWalkAlways — walk vhost ALWAYS (the §4.3 INCLUDE arm AND the
	// AMEND-5 legacy force-include override).
	vhostWalkAlways
	// vhostWalkNever — walk vhost NEVER (the §4.3 IGNORE arm).
	vhostWalkNever
)

// ----------------------------------------------------------------------------
// descriptorInputs — the engine's per-request input bundle.
//
// 24.2 EXTENDS the 24.1 5-arg `buildDescriptors` signature with the four NEW
// inputs needed by the 5 remaining actions:
//   - nodeServiceCluster — the Envoy NODE's service-cluster name (the
//     source_cluster action's value source; parent §4.1 row 1)
//   - rawQuery           — the request's URL query string (without the leading
//     '?'); the query_parameters + query_parameter_value_match actions consume
//     this via url.ParseQuery
//   - dynamicMetadata    — the per-stream dynamic-metadata *Bucket (the
//     metadata action's MetadataSource_DYNAMIC=0 value source per D-RL8)
//   - routeMetadata      — the matched route's *corev3.Metadata (the metadata
//     action's MetadataSource_ROUTE_ENTRY=1 value source per D-RL8; threaded
//     via the NEW DELTA-2 RouteMetadata() callback added at this Task)
//
// All four inputs are nil-tolerant — the engine treats missing values as
// "absent" per the parent §4.1 row table's drop discipline.
// ----------------------------------------------------------------------------

type descriptorInputs struct {
	// routeRateLimits is the matched route's RouteAction.rate_limits per Task
	// 5 (DELTA-2) chain-seeded slice; per D-RL6 this is the Axis-A embedded
	// list at 24.1 (`RateLimitPerRoute.rate_limits` lands at 24.2).
	routeRateLimits []*routev3.RateLimit

	// vhostRateLimits is the matched route's parent VirtualHost.rate_limits
	// per Task 5; consumed only when routeRateLimits is empty (OVERRIDE
	// default at 24.1).
	vhostRateLimits []*routev3.RateLimit

	// headers is the request headers (case-insensitive lookup via
	// http.Header.Get for the request_headers + header_value_match actions).
	headers http.Header

	// remoteAddr is the downstream peer net.Addr per the ADR-0165 callback
	// (DownstreamRemoteAddr); a nil OR non-IP address drops any descriptor
	// whose action chain contains remote_address OR masked_remote_address.
	remoteAddr net.Addr

	// clusterName is the matched route's routed cluster name (the
	// destination_cluster action's value source). Task 7 threads this from
	// the matched routeEntry; the engine takes it as a parameter to remain
	// pure / unit-testable. At 24.1 the decode_headers.go path passes "" —
	// the destination_cluster action then drops the descriptor (documented
	// at decode_headers.go).
	clusterName string

	// nodeServiceCluster is the Envoy NODE's service-cluster name (the
	// source_cluster action's value source per parent §4.1 row 1 + AMEND-11).
	// Threaded from FactoryCtx.NodeServiceCluster via the compiledConfig.
	// Empty string is a defensible production zero-value (the action emits an
	// EMPTY-VALUE entry per upstream rl.cc:89-90 — never drops; see
	// actionSourceCluster).
	nodeServiceCluster string

	// rawQuery is the request URL query string (without the leading '?'). The
	// query_parameters + query_parameter_value_match actions parse this via
	// url.ParseQuery. Empty when the request has no query string OR when the
	// caller did not extract it from the :path pseudo-header.
	rawQuery string

	// dynamicMetadata is the per-stream cross-filter dynamic-metadata Bucket
	// (the metadata action's MetadataSource_DYNAMIC=0 value source per D-RL8).
	// Nil-tolerant: a nil bucket on the metadata-action path is treated as
	// "absent" per the parent §4.1 row 8 drop discipline.
	dynamicMetadata *dynamicmetadata.Bucket

	// routeMetadata is the matched route's *corev3.Metadata (the metadata
	// action's MetadataSource_ROUTE_ENTRY=1 value source per D-RL8). Threaded
	// via the NEW DELTA-2 DecoderFilterCallbacks.RouteMetadata() accessor
	// added at this Task per the ADR-0165 set-once-by-dispatch + ADR-0198
	// chain-field plumbing template. Nil-tolerant: a nil metadata on the
	// metadata-action ROUTE_ENTRY path is treated as "absent".
	routeMetadata *corev3.Metadata

	// filterStage selects the route/vhost RateLimit-policy stage bucket the
	// engine walks at request time per parent §4.4 (upstream
	// `getApplicableRateLimit(stage)`). Threaded from `compiledConfig.stage`
	// via `cc.getStage()` (the FILTER's configured stage; default 0 — the
	// proto-zero default). Policies whose `Stage.Value` differs from
	// `filterStage` are SKIPPED. Default 0 preserves the 24.1 stage-0-only
	// baseline behavior; non-zero filter stages were rejected at parse-time
	// by 24.1's `buildCompiledConfig` arm if > maxStage (10).
	filterStage uint32

	// vhostWalkMode — the §4.3 Axis-B vhost-walk disposition (phase-24.2 Task
	// 4; parent SPEC §4.3 + AMEND-5). Zero-value `vhostWalkOverrideDefault`
	// preserves the 24.1 baseline (walk vhost only when route is empty);
	// `vhostWalkAlways` forces vhost walk (INCLUDE + legacy force-include);
	// `vhostWalkNever` skips vhost entirely (IGNORE). The disposition is
	// computed by decode_headers.go from `RateLimitPerRoute.vh_rate_limits` +
	// the legacy `RouteAction.include_vh_rate_limits` bool BEFORE invoking
	// the engine; the engine just honors the chosen disposition.
	vhostWalkMode vhostWalkMode
}

// ----------------------------------------------------------------------------
// buildDescriptors — the 24.1 backwards-compatible 5-arg entry. Wraps
// buildDescriptorsExt with zero values for the new 24.2 inputs. Preserved so
// the 24.1 unit tests + the d-core differential fixture do not re-bind to
// the new struct shape.
// ----------------------------------------------------------------------------

func buildDescriptors(
	routeRateLimits []*routev3.RateLimit,
	vhostRateLimits []*routev3.RateLimit,
	headers http.Header,
	remoteAddr net.Addr,
	clusterName string,
) []*ratelimitv3.RateLimitDescriptor {
	return buildDescriptorsExt(descriptorInputs{
		routeRateLimits: routeRateLimits,
		vhostRateLimits: vhostRateLimits,
		headers:         headers,
		remoteAddr:      remoteAddr,
		clusterName:     clusterName,
	})
}

// ----------------------------------------------------------------------------
// buildDescriptorsExt — the FULL §4 descriptor-action engine entry-point.
//
// Per D-RL6 the 24.1 walk order honors the Axis-A early-return + OVERRIDE-
// default vhost walk: when the route has a non-empty rate_limits[], only the
// route is walked; otherwise the vhost (if any) is walked.
//
// Output: []*ratelimitv3.RateLimitDescriptor with entries in action-list order
// per AMEND-6. An empty result is a normal outcome (caller short-circuits to
// Continue without an RLS call per parent SPEC §4.6).
//
// Stage filtering (parent §4.4 — phase-24.2 Task 2 multi-stage path): the
// engine evaluates ONLY the policies whose `RateLimit.Stage.Value` equals
// `in.filterStage` (the FILTER's configured stage; default 0). Equivalent to
// the upstream `getApplicableRateLimit(stage)` table-lookup at request time
// (rl.cc:539-550 + parent §4.4). 24.1's default stage-0 baseline behavior is
// preserved when `in.filterStage == 0` (the proto-zero default).
// ----------------------------------------------------------------------------

func buildDescriptorsExt(in descriptorInputs) []*ratelimitv3.RateLimitDescriptor {
	// Phase-24.2 Task 4 (D-RL11 / §4.3 + AMEND-5): the FULL Axis-B vhost-walk
	// composition table. The route policy is walked unconditionally (the
	// §4.3 column "route rate_limits walked?" is YES in EVERY row of the
	// decision table; an empty routeRateLimits[] is a no-op walk). The vhost
	// policy is walked according to `in.vhostWalkMode`:
	//
	//   - vhostWalkOverrideDefault (zero-value): walk vhost ONLY when route
	//     is empty (the §4.3 OVERRIDE arm + 24.1 baseline preserved by the
	//     `buildDescriptors` 5-arg wrapper).
	//   - vhostWalkAlways: walk vhost ALWAYS (the §4.3 INCLUDE arm + the
	//     AMEND-5 legacy `RouteAction.include_vh_rate_limits=true` force-
	//     include override; the legacy bool trumps the enum at the caller).
	//   - vhostWalkNever: walk vhost NEVER (the §4.3 IGNORE arm).
	//
	// Axis-A (D-RL11) is handled at the caller (decode_headers.go) by
	// substituting `in.routeRateLimits` with `RateLimitPerRoute.rate_limits[]`
	// AND zeroing `in.vhostRateLimits` — the engine then walks ONLY the per-
	// route list and the vhost-walk decision is moot (nil vhost regardless of
	// vhostWalkMode produces zero vhost descriptors).
	walkVhost := false
	switch in.vhostWalkMode {
	case vhostWalkAlways:
		walkVhost = true
	case vhostWalkNever:
		walkVhost = false
	case vhostWalkOverrideDefault:
		fallthrough
	default:
		// OVERRIDE-default: walk vhost only when route is empty.
		walkVhost = len(in.routeRateLimits) == 0
	}

	// Pre-size the output for the worst case (route + vhost both walked).
	capHint := len(in.routeRateLimits)
	if walkVhost {
		capHint += len(in.vhostRateLimits)
	}
	if capHint == 0 {
		return nil
	}
	out := make([]*ratelimitv3.RateLimitDescriptor, 0, capHint)

	// Walk route policies first (AMEND-6 policy-order discipline; the route
	// tier is logically more-specific than the vhost tier so its descriptors
	// precede the vhost's in the outbound RateLimitRequest).
	out = appendDescriptors(out, in.routeRateLimits, in)
	if walkVhost {
		out = appendDescriptors(out, in.vhostRateLimits, in)
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// appendDescriptors walks `policies` and appends one descriptor per surviving
// policy to `out`. The §4.4 stage filter + §4.5 whole-descriptor-drop
// discipline apply per policy; nil entries + stage mismatches + action-drops
// are silently skipped. Pure: no per-policy state leaks across calls.
func appendDescriptors(out []*ratelimitv3.RateLimitDescriptor, policies []*routev3.RateLimit, in descriptorInputs) []*ratelimitv3.RateLimitDescriptor {
	for _, p := range policies {
		if p == nil {
			continue
		}
		// §4.4 stage filtering: evaluate ONLY policies in the bucket matching
		// the filter's configured stage. Per-policy stage != filterStage ⇒
		// skip. `p.GetStage()` is `*wrapperspb.UInt32Value`; .GetValue() is
		// nil-safe (returns 0 for absent ≡ stage-0 bucket). The default-0
		// filter stage walks the stage-0 bucket — same as 24.1 baseline.
		if p.GetStage().GetValue() != in.filterStage {
			continue
		}
		desc, ok := buildDescriptorForPolicy(p, in)
		if !ok {
			// §4.5 behavior (1): action returned false ⇒ WHOLE descriptor
			// dropped; loop continues to the next policy (each policy
			// produces its own descriptor independently).
			continue
		}
		out = append(out, desc)
	}
	return out
}

// ----------------------------------------------------------------------------
// buildDescriptorForPolicy — per-policy descriptor build (the §4.5 loop).
// ----------------------------------------------------------------------------

// buildDescriptorForPolicy applies the policy's actions[] in order against
// the request inputs and returns:
//   - (desc, true)  on success (descriptor with entries in action-list order)
//   - (nil,  false) on §4.5 behavior (1) — an action returned false; the
//     WHOLE descriptor is dropped and the actions loop is broken.
//
// §4.5 behavior (2) — empty-key skip — is implemented inline: an action that
// returns (zero-Entry, true, false) is silently skipped while the descriptor
// continues to accumulate entries from the remaining actions.
func buildDescriptorForPolicy(
	p *routev3.RateLimit,
	in descriptorInputs,
) (*ratelimitv3.RateLimitDescriptor, bool) {
	actions := p.GetActions()
	entries := make([]*ratelimitv3.RateLimitDescriptor_Entry, 0, len(actions))
	for _, a := range actions {
		if a == nil {
			continue
		}
		entry, ok, drop := applyAction(a, in)
		if drop {
			// §4.5 (1): the WHOLE descriptor is dropped + the loop breaks.
			return nil, false
		}
		if !ok {
			// §4.5 (2): empty-key skip — this single entry is not appended;
			// the descriptor survives + the loop continues.
			continue
		}
		entries = append(entries, entry)
	}
	return &ratelimitv3.RateLimitDescriptor{Entries: entries}, true
}

// ----------------------------------------------------------------------------
// applyAction — the §4.1 per-action dispatch.
//
// Returns:
//   - entry — the descriptor entry produced (key + value); valid iff ok==true
//   - ok    — true if the action produced an entry to append; false signals
//     either "empty-key skip" (drop==false) or "whole-descriptor drop"
//     (drop==true)
//   - drop  — true ⇒ §4.5 behavior (1): drop the WHOLE descriptor + break
//     the action loop
// ----------------------------------------------------------------------------

// applyAction dispatches one *routev3.RateLimit_Action to its per-action
// helper. At 24.2 ALL 10 canonical actions are wired (the 5 CORE landed at
// 24.1 + the 5 REMAINING landed at this Task per parent §4.1's complete row
// table). The §4.5 empty-action-drop discipline (TWO behaviors) is honored
// uniformly across all 10 arms.
func applyAction(
	a *routev3.RateLimit_Action,
	in descriptorInputs,
) (entry *ratelimitv3.RateLimitDescriptor_Entry, ok bool, drop bool) {
	switch spec := a.GetActionSpecifier().(type) {
	// ---- The 5 CORE actions (landed at 24.1) ----
	case *routev3.RateLimit_Action_GenericKey_:
		return actionGenericKey(spec.GenericKey)
	case *routev3.RateLimit_Action_RequestHeaders_:
		return actionRequestHeaders(spec.RequestHeaders, in.headers)
	case *routev3.RateLimit_Action_RemoteAddress_:
		return actionRemoteAddress(in.remoteAddr)
	case *routev3.RateLimit_Action_DestinationCluster_:
		return actionDestinationCluster(in.clusterName)
	case *routev3.RateLimit_Action_HeaderValueMatch_:
		return actionHeaderValueMatch(spec.HeaderValueMatch, in.headers)

	// ---- The 5 REMAINING actions (landed at 24.2 Task 1) ----
	case *routev3.RateLimit_Action_SourceCluster_:
		return actionSourceCluster(in.nodeServiceCluster)
	case *routev3.RateLimit_Action_MaskedRemoteAddress_:
		return actionMaskedRemoteAddress(spec.MaskedRemoteAddress, in.remoteAddr)
	case *routev3.RateLimit_Action_Metadata:
		return actionMetadata(spec.Metadata, in.dynamicMetadata, in.routeMetadata)
	case *routev3.RateLimit_Action_QueryParameters_:
		return actionQueryParameters(spec.QueryParameters, in.rawQuery)
	case *routev3.RateLimit_Action_QueryParameterValueMatch_:
		return actionQueryParameterValueMatch(spec.QueryParameterValueMatch, in.rawQuery)

	// ---- PARSE-time-rejected actions (§5.2 + ADR-0200; defensive arms) ----
	//
	// These should NEVER reach the engine (ValidateRouteRateLimits at HCM
	// parse-time rejects them per Task 3 / ADR-0200). The defensive whole-
	// descriptor drop here is a belt-and-suspenders for any test path that
	// bypasses HCM validation.
	case *routev3.RateLimit_Action_Extension,
		*routev3.RateLimit_Action_DynamicMetadata:
		return nil, false, true

	// ---- Truly unknown arm — defensive whole-descriptor drop. ----
	default:
		return nil, false, true
	}
}

// ----------------------------------------------------------------------------
// Per-action helpers (the 5 CORE actions).
// ----------------------------------------------------------------------------

// actionGenericKey implements the generic_key action per parent §4.1 row 6:
//   - entry key   = descriptor_key (default "generic_key" per AMEND-11)
//   - entry value = descriptor_value (REQUIRED — empty value drops the
//     descriptor per upstream router_ratelimit.cc:163, 166-183; 24.1 does not
//     honor the proto's default_value field — that arm lands at 24.2 if/when
//     a config exercises it).
//
// Note: a nil action body is treated as drop (defensive — proto unmarshal
// should yield a non-nil GenericKey when the oneof arm is set).
func actionGenericKey(
	gk *routev3.RateLimit_Action_GenericKey,
) (*ratelimitv3.RateLimitDescriptor_Entry, bool, bool) {
	if gk == nil {
		return nil, false, true
	}
	value := gk.GetDescriptorValue()
	if value == "" {
		// Whole-descriptor drop per upstream — empty value AND no default_value.
		return nil, false, true
	}
	key := gk.GetDescriptorKey()
	if key == "" {
		key = descriptorKeyGenericKeyDefault
	}
	return &ratelimitv3.RateLimitDescriptor_Entry{Key: key, Value: value}, true, false
}

// actionRequestHeaders implements the request_headers action per parent §4.1
// row 3:
//   - entry key   = descriptor_key (REQUIRED — no default per AMEND-11)
//   - entry value = first header value for header_name (case-insensitive lookup
//     via http.Header.Get per Go convention)
//   - drop discipline:
//   - header_name OR descriptor_key empty ⇒ drop (defensive — PARSE-time
//     should not pass an empty header_name; an empty descriptor_key with
//     no default is a config-author bug)
//   - header absent + skip_if_absent=false ⇒ drop WHOLE descriptor
//   - header absent + skip_if_absent=true ⇒ skip ONE entry, descriptor
//     survives
//   - header present + empty value ⇒ skip ONE entry (the upstream
//     router_ratelimit.cc:34-36 empty-key/value skip discipline)
func actionRequestHeaders(
	rh *routev3.RateLimit_Action_RequestHeaders,
	headers http.Header,
) (*ratelimitv3.RateLimitDescriptor_Entry, bool, bool) {
	if rh == nil {
		return nil, false, true
	}
	headerName := rh.GetHeaderName()
	descKey := rh.GetDescriptorKey()
	if headerName == "" || descKey == "" {
		// Defensive whole-drop — PARSE-time should not pass either empty.
		return nil, false, true
	}
	// Case-insensitive lookup. http.Header.Get canonicalizes the key.
	value := headers.Get(headerName)
	if value == "" {
		if rh.GetSkipIfAbsent() {
			// Skip ONE entry; descriptor survives.
			return nil, false, false
		}
		// Whole-descriptor drop.
		return nil, false, true
	}
	return &ratelimitv3.RateLimitDescriptor_Entry{Key: descKey, Value: value}, true, false
}

// actionRemoteAddress implements the remote_address action per parent §4.1
// row 4:
//   - entry key   = fixed "remote_address" (AMEND-11)
//   - entry value = downstream remote IP as a string (no port; IPv4 dotted-
//     quad / IPv6 bare form per net.IP.String() convention — matches the
//     extauthz addressFromNetAddr precedent at
//     internal/filter/http/extauthz/attributes.go:605)
//   - drop discipline: nil addr OR non-IP addr ⇒ drop (the extauthz
//     SplitHostPort best-effort fallback is INTENTIONALLY omitted at 24.1 —
//     production seeds *net.TCPAddr exclusively per the ADR-0165 plumbing;
//     test paths that pass non-TCP addresses fail closed at the engine).
func actionRemoteAddress(
	addr net.Addr,
) (*ratelimitv3.RateLimitDescriptor_Entry, bool, bool) {
	if addr == nil {
		return nil, false, true
	}
	ipStr := ipStringFromAddr(addr)
	if ipStr == "" {
		return nil, false, true
	}
	return &ratelimitv3.RateLimitDescriptor_Entry{
		Key:   descriptorKeyRemoteAddress,
		Value: ipStr,
	}, true, false
}

// ipStringFromAddr extracts the bare IP string from a net.Addr. Mirrors the
// ADR-0165 set-once primitive's contract (HCM dispatch seeds *net.TCPAddr).
// Returns "" for nil or non-TCPAddr types — production HCM dispatch ONLY ever
// seeds *net.TCPAddr per ADR-0165's H1/H2 connection-side typed source.
func ipStringFromAddr(a net.Addr) string {
	if v, ok := a.(*net.TCPAddr); ok {
		if v.IP == nil {
			return ""
		}
		return v.IP.String()
	}
	return ""
}

// actionDestinationCluster implements the destination_cluster action per
// parent §4.1 row 2:
//   - entry key   = fixed "destination_cluster" (AMEND-11)
//   - entry value = the matched routeEntry's cluster name (passed in by the
//     caller; Task 7 threads this from the matched routeEntry — the engine
//     is pure and does not look it up).
//   - drop discipline: empty cluster name ⇒ drop (rare — the matched
//     routeEntry always has a cluster name in practice).
func actionDestinationCluster(
	clusterName string,
) (*ratelimitv3.RateLimitDescriptor_Entry, bool, bool) {
	if clusterName == "" {
		return nil, false, true
	}
	return &ratelimitv3.RateLimitDescriptor_Entry{
		Key:   descriptorKeyDestinationCluster,
		Value: clusterName,
	}, true, false
}

// actionHeaderValueMatch implements the header_value_match action per parent
// §4.1 row 7:
//   - entry key   = descriptor_key (default "header_match" per AMEND-11)
//   - entry value = descriptor_value (REQUIRED)
//   - match evaluation: ALL matchers in headers[] must match (vacuous AND-fold
//     — empty list ⇒ matched=true; upstream HeaderUtility::matchHeaders)
//   - drop discipline: matched? != expect_match (default true) ⇒ drop WHOLE
//     descriptor (action returns false). When expect_match is true (default)
//     AND the headers match, OR when expect_match is false AND the headers
//     do NOT match, the entry is emitted.
func actionHeaderValueMatch(
	hvm *routev3.RateLimit_Action_HeaderValueMatch,
	headers http.Header,
) (*ratelimitv3.RateLimitDescriptor_Entry, bool, bool) {
	if hvm == nil {
		return nil, false, true
	}
	value := hvm.GetDescriptorValue()
	if value == "" {
		// Whole-descriptor drop (mirrors generic_key — empty value at this arm).
		return nil, false, true
	}
	// Vacuous AND-fold: empty matchers list ⇒ matched = true.
	matched := evaluateAllHeaderMatchers(hvm.GetHeaders(), headers)
	expectMatch := true
	if em := hvm.GetExpectMatch(); em != nil {
		expectMatch = em.GetValue()
	}
	if matched != expectMatch {
		// Whole-descriptor drop per §4.5 behavior (1).
		return nil, false, true
	}
	key := hvm.GetDescriptorKey()
	if key == "" {
		key = descriptorKeyHeaderValueMatchDefault
	}
	return &ratelimitv3.RateLimitDescriptor_Entry{Key: key, Value: value}, true, false
}

// ----------------------------------------------------------------------------
// Header-matcher evaluation — the subset of HeaderMatcher specifiers used by
// the header_value_match action. Mirrors the oauth2 compileHeaderMatcher
// precedent at internal/filter/http/oauth2/compiled_config.go:864 with two
// differences:
//
//   1. Per-request evaluation (no pre-compile step): the engine is pure +
//      per-request; pre-compiling matchers at HCM parse-time would require
//      threading compiled state through the chain seed (out of scope for
//      24.1 — adds chain plumbing the Task-5 D-RL1 byte-confirmation has not
//      sanctioned). 24.2 may extract a pre-compile path if the per-request
//      regexp.Compile cost shows up in profiling.
//   2. ALL matchers must match (AND-fold), not ANY (the oauth2 list-of-matchers
//      OR-fold). Matches upstream HeaderUtility::matchHeaders semantics.
// ----------------------------------------------------------------------------

// evaluateAllHeaderMatchers returns true iff EVERY HeaderMatcher in matchers
// matches the request headers (vacuous-true AND-fold — empty list returns
// true). The header lookup is case-insensitive via http.Header.Get.
func evaluateAllHeaderMatchers(matchers []*routev3.HeaderMatcher, headers http.Header) bool {
	for _, m := range matchers {
		if !evaluateOneHeaderMatcher(m, headers) {
			return false
		}
	}
	return true
}

// evaluateOneHeaderMatcher returns true iff the single HeaderMatcher's
// specifier (after invert_match application) matches the request header at
// matcher.Name. Supports the same subset as the oauth2 precedent: exact /
// prefix / suffix / contains / safe_regex / present / string_match. Unknown
// or nil matchers evaluate to false (defensive — PARSE-time guards apply
// upstream of HCM seeding).
func evaluateOneHeaderMatcher(m *routev3.HeaderMatcher, headers http.Header) bool {
	if m == nil || m.GetName() == "" {
		return false
	}
	value := headers.Get(m.GetName())
	matched := false
	switch spec := m.GetHeaderMatchSpecifier().(type) {
	case *routev3.HeaderMatcher_ExactMatch:
		matched = value == spec.ExactMatch //nolint:staticcheck // intentional: deprecated arm honored for backward-compat per ADR-0080
	case *routev3.HeaderMatcher_PrefixMatch:
		matched = strings.HasPrefix(value, spec.PrefixMatch) //nolint:staticcheck // intentional: deprecated arm honored for backward-compat per ADR-0080
	case *routev3.HeaderMatcher_SuffixMatch:
		matched = strings.HasSuffix(value, spec.SuffixMatch) //nolint:staticcheck // intentional: deprecated arm honored for backward-compat per ADR-0080
	case *routev3.HeaderMatcher_ContainsMatch:
		matched = strings.Contains(value, spec.ContainsMatch) //nolint:staticcheck // intentional: deprecated arm honored for backward-compat per ADR-0080
	case *routev3.HeaderMatcher_SafeRegexMatch:
		safeRe := spec.SafeRegexMatch //nolint:staticcheck // intentional: deprecated arm honored for backward-compat per ADR-0080
		if safeRe == nil {
			matched = false
		} else if re, err := regexp.Compile(safeRe.GetRegex()); err == nil {
			matched = re.MatchString(value)
		}
	case *routev3.HeaderMatcher_PresentMatch:
		present := value != ""
		if spec.PresentMatch {
			matched = present
		} else {
			matched = !present
		}
	case *routev3.HeaderMatcher_StringMatch:
		matched = evaluateStringMatcher(spec.StringMatch, value)
	}
	if m.GetInvertMatch() {
		return !matched
	}
	return matched
}

// evaluateStringMatcher implements the StringMatcher arms used by the
// header_value_match action's StringMatch specifier. Subset matches the
// oauth2 compileStringMatcher precedent. Case-insensitive variants honor
// the StringMatcher.IgnoreCase bool.
//
// Supported arms: Exact / Prefix / Suffix / Contains / SafeRegex. The
// Custom arm (TypedExtensionConfig) is NOT supported at 24.1 — falls
// through to false (no match) defensively; 24.2 may extend if a config
// surfaces it.
func evaluateStringMatcher(sm *matcherv3.StringMatcher, value string) bool {
	if sm == nil {
		return false
	}
	ignoreCase := sm.GetIgnoreCase()
	// For case-insensitive comparison, lower-case both sides for the
	// Exact/Prefix/Suffix/Contains arms (SafeRegex ignores IgnoreCase per
	// the proto doc).
	cmpValue := value
	if ignoreCase {
		cmpValue = strings.ToLower(value)
	}
	switch spec := sm.GetMatchPattern().(type) {
	case *matcherv3.StringMatcher_Exact:
		want := spec.Exact
		if ignoreCase {
			want = strings.ToLower(want)
		}
		return cmpValue == want
	case *matcherv3.StringMatcher_Prefix:
		want := spec.Prefix
		if ignoreCase {
			want = strings.ToLower(want)
		}
		return strings.HasPrefix(cmpValue, want)
	case *matcherv3.StringMatcher_Suffix:
		want := spec.Suffix
		if ignoreCase {
			want = strings.ToLower(want)
		}
		return strings.HasSuffix(cmpValue, want)
	case *matcherv3.StringMatcher_Contains:
		want := spec.Contains
		if ignoreCase {
			want = strings.ToLower(want)
		}
		return strings.Contains(cmpValue, want)
	case *matcherv3.StringMatcher_SafeRegex:
		if spec.SafeRegex == nil {
			return false
		}
		re, err := regexp.Compile(spec.SafeRegex.GetRegex())
		if err != nil {
			return false
		}
		return re.MatchString(value)
	}
	return false
}

// ----------------------------------------------------------------------------
// Per-action helpers (the 5 REMAINING actions — landed at phase-24.2 Task 1).
// ----------------------------------------------------------------------------

// actionSourceCluster implements the source_cluster action per parent §4.1
// row 1 (upstream rl.cc:89-90):
//   - entry key   = literal "source_cluster" (AMEND-11)
//   - entry value = the filter's local_service_cluster (the Envoy NODE's
//     service-cluster name), threaded through descriptorInputs from
//     compiledConfig.nodeServiceCluster
//   - drop discipline: ALWAYS-TRUE. The upstream router_ratelimit.cc:89-90
//     populates the descriptor entry unconditionally; an empty
//     local_service_cluster produces a (key="source_cluster", value="") entry
//     and the descriptor survives. This mirrors the §4.5 behavior (2)
//     "empty-VALUE entry" path — the KEY is non-empty so the push_back fires.
func actionSourceCluster(
	nodeServiceCluster string,
) (*ratelimitv3.RateLimitDescriptor_Entry, bool, bool) {
	return &ratelimitv3.RateLimitDescriptor_Entry{
		Key:   descriptorKeySourceCluster,
		Value: nodeServiceCluster,
	}, true, false
}

// actionMaskedRemoteAddress implements the masked_remote_address action per
// parent §4.1 row 5 (upstream rl.cc:141, 154-156):
//   - entry key   = literal "masked_remote_address" (AMEND-11)
//   - entry value = "<masked-ip>/<mask-len>" — the downstream IP masked with
//     v4_prefix_mask_len for IPv4 (default 32) or v6_prefix_mask_len for IPv6
//     (default 128)
//   - drop discipline: nil OR non-IP downstream addr ⇒ drop WHOLE descriptor
//     (parallel to remote_address — production HCM dispatch seeds *net.TCPAddr
//     exclusively per ADR-0165)
//
// Masking semantics: net.IP.Mask + net.CIDRMask. Mask len bounds-check: for
// IPv4 [0,32]; for IPv6 [0,128]. Out-of-range mask lens are an upstream
// validation concern; defensively clamped here to the family's max.
func actionMaskedRemoteAddress(
	mra *routev3.RateLimit_Action_MaskedRemoteAddress,
	addr net.Addr,
) (*ratelimitv3.RateLimitDescriptor_Entry, bool, bool) {
	if addr == nil {
		return nil, false, true
	}
	tcp, ok := addr.(*net.TCPAddr)
	if !ok || tcp.IP == nil {
		return nil, false, true
	}
	ip := tcp.IP

	// Determine v4 vs v6 family. net.IP.To4() returns non-nil for IPv4 (incl.
	// IPv4-mapped-IPv6). For IPv6, use the 16-byte form.
	var maskLen uint32
	var ipFamilyBits int
	if v4 := ip.To4(); v4 != nil {
		ip = v4
		ipFamilyBits = 32
		maskLen = defaultV4PrefixMaskLen
		if mra != nil && mra.GetV4PrefixMaskLen() != nil {
			maskLen = mra.GetV4PrefixMaskLen().GetValue()
		}
	} else {
		ipFamilyBits = 128
		maskLen = defaultV6PrefixMaskLen
		if mra != nil && mra.GetV6PrefixMaskLen() != nil {
			maskLen = mra.GetV6PrefixMaskLen().GetValue()
		}
	}
	// Defensive clamp: if proto carries a mask-len exceeding the family's max,
	// clamp to the max so net.CIDRMask does not return nil (which would
	// produce a wrong masked value).
	if int(maskLen) > ipFamilyBits {
		maskLen = uint32(ipFamilyBits)
	}

	mask := net.CIDRMask(int(maskLen), ipFamilyBits)
	if mask == nil {
		// Defensive — net.CIDRMask only returns nil on out-of-range bits, which
		// we have just clamped against. Belt-and-suspenders.
		return nil, false, true
	}
	masked := ip.Mask(mask)
	value := fmt.Sprintf("%s/%d", masked.String(), maskLen)
	return &ratelimitv3.RateLimitDescriptor_Entry{
		Key:   descriptorKeyMaskedRemoteAddress,
		Value: value,
	}, true, false
}

// actionMetadata implements the metadata action per parent §4.1 row 8 (upstream
// rl.cc:187-227 + source/common/config/metadata.cc::Metadata::metadataValue):
//   - entry key   = descriptor_key (REQUIRED — no default per AMEND-11)
//   - entry value = MetadataKey lookup result (descend MetadataKey.path[] over
//     the structpb chain) OR default_value if absent
//   - drop discipline:
//   - descriptor_key empty ⇒ drop (defensive; AMEND-11 REQUIRES descriptor_key)
//   - present (string leaf) ⇒ emit value
//   - present (non-string leaf) ⇒ treat as absent (upstream metadataValue
//     requires Kind==STRING; non-string leaves fall through to absent)
//   - absent + default_value present ⇒ emit default_value
//   - absent + no default + skip_if_absent=false ⇒ drop WHOLE descriptor
//   - absent + no default + skip_if_absent=true  ⇒ skip ONE entry (descriptor survives)
//
// Source dispatch (D-RL8):
//   - MetadataSource_DYNAMIC (0)     — dynamic-metadata bucket lookup at
//     (MetadataKey.key, MetadataKey.path[0].key) → descend path[1:]
//   - MetadataSource_ROUTE_ENTRY (1) — matched route's *corev3.Metadata
//     lookup at filter_metadata[MetadataKey.key] → descend path[]
//
// Both source paths share the segmented descent helper lookupMetadataValue
// below.
func actionMetadata(
	md *routev3.RateLimit_Action_MetaData,
	dynBucket *dynamicmetadata.Bucket,
	routeMD *corev3.Metadata,
) (*ratelimitv3.RateLimitDescriptor_Entry, bool, bool) {
	if md == nil {
		return nil, false, true
	}
	descKey := md.GetDescriptorKey()
	if descKey == "" {
		// AMEND-11: descriptor_key is REQUIRED; defensive whole-descriptor drop.
		return nil, false, true
	}

	value, found := resolveMetadataValue(md, dynBucket, routeMD)
	if !found {
		// Apply default_value / skip_if_absent discipline.
		if def := md.GetDefaultValue(); def != "" {
			return &ratelimitv3.RateLimitDescriptor_Entry{
				Key:   descKey,
				Value: def,
			}, true, false
		}
		if md.GetSkipIfAbsent() {
			// §4.5 behavior (2): skip ONE entry; descriptor survives.
			return nil, false, false
		}
		// §4.5 behavior (1): drop WHOLE descriptor.
		return nil, false, true
	}
	return &ratelimitv3.RateLimitDescriptor_Entry{
		Key:   descKey,
		Value: value,
	}, true, false
}

// resolveMetadataValue dispatches the MetadataSource arm and returns
// (string-value, found). Non-string leaves return (_, false) per upstream
// metadataValue (Kind==STRING requirement). MetadataKey shape:
//
//	MetadataKey {
//	  key  : "<filter-name>"                  // top-level filter_metadata key
//	  path : [ {key: "<seg-1>"}, ... ]        // descent into the value
//	}
//
// The descent semantics depend on the source:
//
//   - DYNAMIC: the Bucket.Get(filterName, path[0].key) returns the top-level
//     *structpb.Value; path[1:] descends through Value.GetStructValue().Fields.
//   - ROUTE_ENTRY: routeMD.FilterMetadata[filterName] is the *structpb.Struct;
//     path[0].key indexes its Fields; path[1:] descends through subsequent
//     Struct/Value children.
//
// DYNAMIC vs ROUTE_ENTRY asymmetry on the (filterName, path[0]) coordinate.
// The DYNAMIC arm consumes envoy-go's pre-existing `*dynamicmetadata.Bucket`,
// whose storage shape is a FLAT `(filterName, key) → *structpb.Value` map
// rather than upstream's `map<string, google.protobuf.Struct>` (one Struct per
// filter). Consequently DYNAMIC decomposes the first two coordinates
// (filterName, path[0]) at the Bucket boundary, while ROUTE_ENTRY descends
// path[0] as the first FIELD of the per-filter Struct. Producers should write
// structurally — `bucket.Set(filterName, topField, value)` where `value` is a
// Struct mirroring the per-filter `Struct.Fields` shape — so the two arms are
// semantically equivalent at the shared `descendStructpbValue` join-point.
//
// Empty path ⇒ absent (defensive — the proto schema does not formally require
// path non-empty, but upstream metadataValue returns Null on empty path).
func resolveMetadataValue(
	md *routev3.RateLimit_Action_MetaData,
	dynBucket *dynamicmetadata.Bucket,
	routeMD *corev3.Metadata,
) (string, bool) {
	mk := md.GetMetadataKey()
	if mk == nil {
		return "", false
	}
	filterName := mk.GetKey()
	segments := mk.GetPath()
	if filterName == "" || len(segments) == 0 {
		return "", false
	}

	// The first path segment is the top-level field name in the filter's
	// struct. Subsequent path segments descend through nested Struct fields.
	firstKey := segments[0].GetKey()
	if firstKey == "" {
		return "", false
	}
	rest := segments[1:]

	switch md.GetSource() {
	case routev3.RateLimit_Action_MetaData_DYNAMIC:
		if dynBucket == nil {
			return "", false
		}
		v, ok := dynBucket.Get(filterName, firstKey)
		if !ok || v == nil {
			return "", false
		}
		return descendStructpbValue(v, rest)

	case routev3.RateLimit_Action_MetaData_ROUTE_ENTRY:
		if routeMD == nil {
			return "", false
		}
		filterStruct, ok := routeMD.GetFilterMetadata()[filterName]
		if !ok || filterStruct == nil {
			return "", false
		}
		v, ok := filterStruct.GetFields()[firstKey]
		if !ok || v == nil {
			return "", false
		}
		return descendStructpbValue(v, rest)

	default:
		return "", false
	}
}

// descendStructpbValue descends `value` through the sequence of segment keys
// in `rest`. Each non-terminal segment requires value.Kind == Struct;
// otherwise the chain breaks and absent is returned. Returns the terminal
// string value (Kind == String) when descent completes successfully; non-
// string terminals return ("", false) per upstream metadataValue
// (Kind==STRING requirement).
func descendStructpbValue(value *structpb.Value, rest []*metadatav3.MetadataKey_PathSegment) (string, bool) {
	cur := value
	for _, seg := range rest {
		segKey := seg.GetKey()
		if segKey == "" {
			return "", false
		}
		// Non-Struct intermediate ⇒ chain breaks.
		st := cur.GetStructValue()
		if st == nil {
			return "", false
		}
		next, ok := st.GetFields()[segKey]
		if !ok || next == nil {
			return "", false
		}
		cur = next
	}
	// Terminal value must be a string per upstream metadataValue semantics.
	switch cur.GetKind().(type) {
	case *structpb.Value_StringValue:
		return cur.GetStringValue(), true
	default:
		return "", false
	}
}

// actionQueryParameters implements the query_parameters action per parent
// §4.1 row 9 (upstream rl.cc:232-253):
//   - entry key   = descriptor_key (default "query_param" SINGULAR per AMEND-11)
//   - entry value = first value of query_parameter_name in the request query
//   - drop discipline:
//   - query_parameter_name empty ⇒ drop (defensive; required by upstream)
//   - param absent + skip_if_absent=false ⇒ drop WHOLE descriptor
//   - param absent + skip_if_absent=true  ⇒ skip ONE entry (descriptor survives)
//
// Query-string parsing: net/url.ParseQuery splits on '&' and decodes percent-
// encoded values. Failure to parse (malformed query) is treated as absent —
// matches the upstream tolerance for relaxed query strings.
func actionQueryParameters(
	qp *routev3.RateLimit_Action_QueryParameters,
	rawQuery string,
) (*ratelimitv3.RateLimitDescriptor_Entry, bool, bool) {
	if qp == nil {
		return nil, false, true
	}
	paramName := qp.GetQueryParameterName()
	if paramName == "" {
		// Defensive: upstream PGV requires query_parameter_name non-empty.
		return nil, false, true
	}

	values, ok := lookupFirstQueryParam(rawQuery, paramName)
	if !ok {
		if qp.GetSkipIfAbsent() {
			// §4.5 behavior (2): skip ONE entry; descriptor survives.
			return nil, false, false
		}
		// §4.5 behavior (1): drop WHOLE descriptor.
		return nil, false, true
	}

	key := qp.GetDescriptorKey()
	if key == "" {
		key = descriptorKeyQueryParametersDefault
	}
	return &ratelimitv3.RateLimitDescriptor_Entry{
		Key:   key,
		Value: values,
	}, true, false
}

// lookupFirstQueryParam returns (firstValueOf(name), ok=true) when name is
// present in rawQuery; ("", false) when absent OR when the query is malformed.
// Mirrors Go's url.ParseQuery behavior (the FIRST occurrence's value when the
// key is repeated — matches upstream rl.cc:232-253 first-value semantics).
func lookupFirstQueryParam(rawQuery, name string) (string, bool) {
	if rawQuery == "" {
		return "", false
	}
	// url.ParseQuery tolerates malformed entries (returns an error AND a
	// partial map). We ignore the error and consult the partial map — matches
	// upstream's relaxed query parsing.
	vals, _ := url.ParseQuery(rawQuery)
	got, ok := vals[name]
	if !ok || len(got) == 0 {
		return "", false
	}
	return got[0], true
}

// actionQueryParameterValueMatch implements the query_parameter_value_match
// action per parent §4.1 row 10 (upstream rl.cc:297, 304-328):
//   - entry key   = descriptor_key (default "query_match" per AMEND-11)
//   - entry value = descriptor_value (REQUIRED)
//   - matchers AND-fold; entry emitted iff matched == expect_match (default true)
//   - drop discipline: descriptor_value empty ⇒ drop; mismatch ⇒ drop
func actionQueryParameterValueMatch(
	qpvm *routev3.RateLimit_Action_QueryParameterValueMatch,
	rawQuery string,
) (*ratelimitv3.RateLimitDescriptor_Entry, bool, bool) {
	if qpvm == nil {
		return nil, false, true
	}
	value := qpvm.GetDescriptorValue()
	if value == "" {
		// Whole-descriptor drop — mirrors generic_key empty-value discipline.
		return nil, false, true
	}
	matched := evaluateAllQueryParameterMatchers(qpvm.GetQueryParameters(), rawQuery)
	expectMatch := true
	if em := qpvm.GetExpectMatch(); em != nil {
		expectMatch = em.GetValue()
	}
	if matched != expectMatch {
		// Whole-descriptor drop per §4.5 behavior (1).
		return nil, false, true
	}
	key := qpvm.GetDescriptorKey()
	if key == "" {
		key = descriptorKeyQueryParameterValueMatchDefault
	}
	return &ratelimitv3.RateLimitDescriptor_Entry{
		Key:   key,
		Value: value,
	}, true, false
}

// evaluateAllQueryParameterMatchers returns true iff EVERY QueryParameterMatcher
// in matchers matches the parsed query string (vacuous-true AND-fold — empty
// list returns true; mirrors evaluateAllHeaderMatchers for header_value_match).
func evaluateAllQueryParameterMatchers(matchers []*routev3.QueryParameterMatcher, rawQuery string) bool {
	if len(matchers) == 0 {
		return true
	}
	// Parse once for all matchers per request.
	vals, _ := url.ParseQuery(rawQuery)
	for _, m := range matchers {
		if !evaluateOneQueryParameterMatcher(m, vals) {
			return false
		}
	}
	return true
}

// evaluateOneQueryParameterMatcher returns true iff the matcher's specifier
// matches against vals[matcher.Name]. Supports PresentMatch + StringMatch
// (Exact/Prefix/Suffix/Contains/SafeRegex) — matches the upstream
// QueryParameterMatcher subset commonly seen in rate-limit configs.
func evaluateOneQueryParameterMatcher(m *routev3.QueryParameterMatcher, vals url.Values) bool {
	if m == nil || m.GetName() == "" {
		return false
	}
	got, present := vals[m.GetName()]
	firstVal := ""
	if present && len(got) > 0 {
		firstVal = got[0]
	}
	switch spec := m.GetQueryParameterMatchSpecifier().(type) {
	case *routev3.QueryParameterMatcher_PresentMatch:
		// PresentMatch true ⇒ present; false ⇒ absent.
		if spec.PresentMatch {
			return present
		}
		return !present
	case *routev3.QueryParameterMatcher_StringMatch:
		// StringMatch requires the parameter to be present AND match.
		if !present {
			return false
		}
		return evaluateStringMatcher(spec.StringMatch, firstVal)
	}
	return false
}
