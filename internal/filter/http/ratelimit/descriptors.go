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
// # Stage filtering
//
// 24.1 evaluates the default stage-0 bucket ONLY. Policies whose RateLimit.Stage
// is non-zero are SKIPPED at the engine. (PARSE-time already rejects the
// filter-level stage > 10 per §5.1 arm 3 — that is the FILTER stage, distinct
// from the per-policy stage; 24.2 will land the multi-stage descriptor build.)
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
	"net"
	"net/http"
	"regexp"
	"strings"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/ratelimit/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
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
)

// ----------------------------------------------------------------------------
// buildDescriptors — the engine entry-point.
// ----------------------------------------------------------------------------

// buildDescriptors is the PURE §4 descriptor-action engine. Per D-RL6 the
// 24.1 walk order honors the Axis-A early-return + OVERRIDE-default vhost
// walk: when the route has a non-empty rate_limits[], only the route is
// walked; otherwise the vhost (if any) is walked.
//
// Inputs (all read-only; the engine performs no I/O):
//   - routeRateLimits  — the matched route's RouteAction.rate_limits per Task 5
//     (DELTA-2) chain-seeded slice; per D-RL6 this is the Axis-A embedded list
//     at 24.1 (`RateLimitPerRoute.rate_limits` lands at 24.2).
//   - vhostRateLimits  — the matched route's parent VirtualHost.rate_limits
//     per Task 5; consumed only when routeRateLimits is empty (OVERRIDE
//     default at 24.1).
//   - headers          — the request headers (case-insensitive lookup via
//     http.Header.Get for the request_headers + header_value_match actions).
//   - remoteAddr       — the downstream peer net.Addr per the ADR-0165
//     callback (DownstreamRemoteAddr); a nil OR non-IP address drops any
//     descriptor whose action chain contains remote_address.
//   - clusterName      — the matched route's routed cluster name (the
//     destination_cluster action's value source). Task 7 threads this from
//     the matched routeEntry; the engine takes it as a parameter to remain
//     pure / unit-testable.
//
// Output: []*ratelimitv3.RateLimitDescriptor with entries in action-list order
// per AMEND-6. An empty result is a normal outcome (caller short-circuits to
// Continue without an RLS call per parent SPEC §4.6).
//
// Stage filtering at 24.1: only the default stage-0 bucket is evaluated;
// policies with RateLimit.Stage > 0 are skipped. 24.2 lands the multi-stage
// path (parent §4.4).
func buildDescriptors(
	routeRateLimits []*routev3.RateLimit,
	vhostRateLimits []*routev3.RateLimit,
	headers http.Header,
	remoteAddr net.Addr,
	clusterName string,
) []*ratelimitv3.RateLimitDescriptor {
	// D-RL6 / §4.3 OVERRIDE-default vhost walk: route is ALWAYS walked when
	// non-empty (Axis-A early-return); vhost is walked ONLY when route is
	// empty. The full Axis-B table (INCLUDE / IGNORE / include_vh_rate_limits)
	// lands at 24.2.
	var policies []*routev3.RateLimit
	if len(routeRateLimits) > 0 {
		policies = routeRateLimits
	} else {
		policies = vhostRateLimits
	}

	if len(policies) == 0 {
		return nil
	}

	out := make([]*ratelimitv3.RateLimitDescriptor, 0, len(policies))
	for _, p := range policies {
		if p == nil {
			continue
		}
		// 24.1 stage filtering: evaluate ONLY the default stage-0 bucket.
		// Per-policy stage != 0 ⇒ skip the policy. Full multi-stage at 24.2.
		// p.Stage is *wrapperspb.UInt32Value — nil OR Value==0 ⇒ stage 0.
		if st := p.GetStage(); st != nil && st.GetValue() != 0 {
			continue
		}
		desc, ok := buildDescriptorForPolicy(p, headers, remoteAddr, clusterName)
		if !ok {
			// §4.5 behavior (1): action returned false ⇒ WHOLE descriptor
			// dropped; loop continues to the next policy (each policy
			// produces its own descriptor independently).
			continue
		}
		out = append(out, desc)
	}
	if len(out) == 0 {
		return nil
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
	headers http.Header,
	remoteAddr net.Addr,
	clusterName string,
) (*ratelimitv3.RateLimitDescriptor, bool) {
	actions := p.GetActions()
	entries := make([]*ratelimitv3.RateLimitDescriptor_Entry, 0, len(actions))
	for _, a := range actions {
		if a == nil {
			continue
		}
		entry, ok, drop := applyAction(a, headers, remoteAddr, clusterName)
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
// helper. The 5 CORE actions return real entries; the remaining 5 actions
// return a clearly-marked "drop until 24.2" outcome (see actionUnsupportedAt241
// below) so a config exercising them at 24.1 fails closed at the engine layer.
func applyAction(
	a *routev3.RateLimit_Action,
	headers http.Header,
	remoteAddr net.Addr,
	clusterName string,
) (entry *ratelimitv3.RateLimitDescriptor_Entry, ok bool, drop bool) {
	switch spec := a.GetActionSpecifier().(type) {
	// ---- The 5 CORE actions implemented at 24.1 ----
	case *routev3.RateLimit_Action_GenericKey_:
		return actionGenericKey(spec.GenericKey)
	case *routev3.RateLimit_Action_RequestHeaders_:
		return actionRequestHeaders(spec.RequestHeaders, headers)
	case *routev3.RateLimit_Action_RemoteAddress_:
		return actionRemoteAddress(remoteAddr)
	case *routev3.RateLimit_Action_DestinationCluster_:
		return actionDestinationCluster(clusterName)
	case *routev3.RateLimit_Action_HeaderValueMatch_:
		return actionHeaderValueMatch(spec.HeaderValueMatch, headers)

	// ---- The 5 actions deferred to 24.2 (NOT silently dropped) ----
	//
	// Each arm here returns drop=true so a config exercising one of these
	// actions at 24.1 fails closed at the engine (the descriptor is dropped;
	// no RLS call carries it). 24.2 lands the per-action helpers for these
	// arms; the dispatch site extends cleanly by replacing each case body
	// with the new helper call.
	case *routev3.RateLimit_Action_SourceCluster_:
		// 24.2: actionSourceCluster reads the node service-cluster name
		// (parent §4.1 row 1; upstream rl.cc:89-90).
		return actionUnsupportedAt241()
	case *routev3.RateLimit_Action_MaskedRemoteAddress_:
		// 24.2: actionMaskedRemoteAddress applies the v4/v6_prefix_mask_len
		// CIDR mask to the downstream IP (parent §4.1 row 5; rl.cc:141, 154-156).
		return actionUnsupportedAt241()
	case *routev3.RateLimit_Action_Metadata:
		// 24.2: actionMetadata reads the metadata_key from the configured
		// source (DYNAMIC=0 / ROUTE_ENTRY=1) (parent §4.1 row 8; rl.cc:187-227).
		// PARSE-time rejects the deprecated `dynamic_metadata` arm per §5.2 +
		// ADR-0200; this is the SUCCESSOR `metadata` arm.
		return actionUnsupportedAt241()
	case *routev3.RateLimit_Action_QueryParameters_:
		// 24.2: actionQueryParameters reads the first value of query_param_name
		// (parent §4.1 row 9; rl.cc:232-253).
		return actionUnsupportedAt241()
	case *routev3.RateLimit_Action_QueryParameterValueMatch_:
		// 24.2: actionQueryParameterValueMatch (parent §4.1 row 10; rl.cc:297,
		// 304-328).
		return actionUnsupportedAt241()

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

// actionUnsupportedAt241 is the shared sentinel for the 5 actions whose
// helpers land at 24.2. Returns drop=true so the descriptor is dropped (NOT
// silently emitted with no entries). The forward-pointer in each case arm
// above names the helper that will replace this sentinel at 24.2.
func actionUnsupportedAt241() (*ratelimitv3.RateLimitDescriptor_Entry, bool, bool) {
	// drop=true ⇒ §4.5 behavior (1): drop WHOLE descriptor + break loop.
	return nil, false, true
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
