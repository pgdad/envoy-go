package rbac

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"

	rbacv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	rbacengine "github.com/pgdad/envoy-go/internal/rbac"
	"github.com/pgdad/envoy-go/internal/stats"
)

// TypeURL is the canonical envoy.filters.http.rbac typed_config type URL.
// Boot wiring at cmd/envoy-go/main.go (Task 11) registers New under this
// key in the HTTPRegistry per ADR-0072 + ADR-0140.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.rbac.v3.RBAC"

// filterName is the canonical http_filters[].name string for rbac
// (matches the listener config typed_per_filter_config map keys).
const filterName = "envoy.filters.http.rbac"

// denyBody is the 19-byte ASCII deny-path body per §1.1 amendment 10 + §11.P5.
// Byte-exact verbatim with reference Envoy v1.37.2's SendLocalReply path.
// Length asserted at Task 7 Group 6 test TestDecodeHeaders_SendLocalReply_Body19Bytes_RBACAccessDenied.
//
//nolint:unused // consumed at Task 7 DecodeHeaders SendLocalReply call.
const denyBody = "RBAC: access denied"

// compiledConfig is the parsed + validated runtime config for one RBAC filter
// instance (listener-level OR per-route). 8-field shape per SPEC §6.2 +
// ADR-0141. Closure-capture parse-at-New + read-only-shared-after-New per
// ADR-0101.
type compiledConfig struct {
	// Primary engine (rules or matcher; mutually exclusive; rules wins if both
	// set per §1.1 amendment 2 + rbac.pb.go:38).
	rules   *rbacengine.CompiledRulesEngine   // nil when matcher set (or both unset → wholly inactive)
	matcher *rbacengine.CompiledMatcherEngine // nil when rules set or wholly inactive

	// Shadow engine (shadow_rules or shadow_matcher; mutually exclusive;
	// shadow_rules wins per §1.1 amendment 2).
	shadowRules   *rbacengine.CompiledRulesEngine
	shadowMatcher *rbacengine.CompiledMatcherEngine

	// Stat namespacing (proto fields; empty default permitted per §1.1
	// amendment 3).
	rulesStatPrefix       string // proto: rules_stat_prefix
	shadowRulesStatPrefix string // proto: shadow_rules_stat_prefix
	trackPerRuleStats     bool   // proto: track_per_rule_stats

	// Stats — nil when ctx.Stats is nil (test path per ADR-0085 nil-tolerance).
	// 4 base counters per active stat_prefix combination per ADR-0140 +
	// ADR-0145; full registration lands at Task 8.
	stats *filterStats
}

// compiledPerRoute wraps the per-route TPFC disposition per §5.1 +
// ADR-0125 §(xii) amendment (anchored at Task 10).
//
// (a) disabled=true / overrideConfig=nil — case (a): RBACPerRoute.rbac is nil
// → wholly disabled on this route (passthrough; no counters).
// (b) disabled=false / overrideConfig!=nil — case (b): RBACPerRoute.rbac
// non-nil → wholesale-override of the listener-level config (per ADR-0073
// most-specific override semantic).
type compiledPerRoute struct {
	disabled       bool            // true when RBACPerRoute.rbac == nil per §5.1 (a)
	overrideConfig *compiledConfig // non-nil when RBACPerRoute.rbac != nil per §5.1 (b); INDEPENDENT-stats per §5.2
}

// factoryState is the closure-captured shared state per factory invocation.
// Mirrors phase-11 ADR-0117 + phase-15 ADR-0139 IMPL-1 pattern per SPEC §6.3.
//
// hcmStatPrefix is captured from FactoryCtx.StatPrefix at New time and threaded
// through to per-route INDEPENDENT-stats allocation at request-time resolve
// (newFilterStatsIfAbsent path) per ADR-0145. Empty when ctx.StatPrefix is
// empty (test code paths); newFilterStats / newFilterStatsIfAbsent fall back
// to a non-HCM-rooted shape in that case to satisfy the Registry name regex.
type factoryState struct {
	listenerRC    *compiledConfig
	perRoute      sync.Map // map[*rbacv3.RBACPerRoute]*compiledPerRoute — per ADR-0117 + ADR-0125 §(xii) 7th canonical
	reg           *stats.Registry
	hcmStatPrefix string // ctx.StatPrefix captured at New time; threaded to per-route stat-allocation per ADR-0145
}

// filter is the per-stream filter instance allocated by the factory closure.
// Decoder-only per ADR-0140 §Decision (iv) + §1 item 5; no encode-side state.
// Mirrors phase-12 csrf + phase-13 buffer decoder-only precedent.
//
// *filter implements rbacengine.EvalContext per CF-6 (Task 5 notes F-1/F-2) — the
// per-stream accessors (Header / URLPath / Method / etc.) consult f.headers
// (and f.dcb for the DownstreamPrincipal accessor). Connection-info accessors
// (DestinationIP/Port, SNI, DirectRemoteIP, RemoteIP) return nil/empty at
// phase-16 MVP pending future framework primitives — see the per-accessor
// doc-comments on the methods at the bottom of rbac.go.
type filter struct {
	state *factoryState
	dcb   envoyhttp.DecoderFilterCallbacks

	// Per-stream state cached at DecodeHeaders. Per SPEC §6.7.
	headers     http.Header     // request headers; consulted by rbacengine.EvalContext accessors
	activeRC    *compiledConfig // resolved at DecodeHeaders; may be listener-level OR per-route overrideConfig
	passthrough bool            // true when per-route disabled=true OR active engines both nil
}

// filterStats is the 4-base-counter + variable per-policy stat surface per
// §1.1 amendment 8 + ADR-0140 + ADR-0145. REFUTES BRAINSTORM 5-counter
// hypothesis: NO `logged` counter exists in Envoy v1.37.2 per §11.P6 — LOG-
// partial folds into `allowed` per amendment 5 + amendment 8.
//
// Counter registration finalized at Task 8 per ADR-0145 — all 4 base counters
// registered unconditionally (predeclared empty counters for scrape stability
// per Prometheus best practice). The full namespace shape (SN2-reuse per
// §11.P7 + amendment 9 — `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.
// <counter>`) was empirically RATIFIED at Task 8 via the reference Envoy
// v1.37.2 stats scrape — NO new SN10 rule introduced.
//
// **Caller MUST guard against nil registry**: newFilterStats /
// newFilterStatsIfAbsent unconditionally dereference reg; the nil-tolerance
// contract lives at the buildCompiledConfig caller (ctx.Stats != nil gate)
// per ADR-0085.
type filterStats struct {
	// 4 base counters (cumulative across all streams). Increments emitted at
	// DecodeHeaders dispatch via emitPrimaryCounters / emitShadowCounters.
	allowed       *stats.Counter // primary engine result = ALLOWED (or LOG-partial folds in per amendment 5)
	denied        *stats.Counter // primary engine result = DENIED
	shadowAllowed *stats.Counter // shadow engine result = ALLOWED (when shadow configured)
	shadowDenied  *stats.Counter // shadow engine result = DENIED

	// Per-policy lazy-cache (allocated only when trackPerRuleStats=true; keyed
	// by the full counter name e.g. `http.<HCM>.rbac.<rules_prefix>.policy.
	// <policy_name>.allowed`). Lazy allocation via (*PerPolicyCounters).Inc →
	// sync.Map LoadOrStore + NewCounterIfAbsent on first match per ADR-0145.
	// The cache type moved to internal/rbac at phase-26.3 Task 5 (D-26.3-7);
	// the consumer holds a pointer + supplies the Registry + base prefix at
	// emit time (byte-identical counter names, R4).
	perPolicy *rbacengine.PerPolicyCounters

	// reg is captured at New time for lazy per-policy NewCounterIfAbsent at
	// request-time match per ADR-0117 post-Freeze idempotency. Per-route
	// allocation path inherits via the factoryState.reg pointer.
	reg *stats.Registry

	// primaryBase + shadowBase cache the per-counter HCM-rooted base prefix
	// (e.g., `http.<HCM>.rbac.<rules_prefix>`) so (*rbacengine.PerPolicyCounters).Inc
	// can construct the per-policy counter name without recomputing the prefix on every call.
	// Per ADR-0145.
	primaryBase string
	shadowBase  string
}

// Statically assert the decoder-only interface conformance. Per ADR-0140
// §Decision (iv); mirrors phase-12 csrf + phase-13 buffer precedent (3rd §9
// row to ship pure decode-side).
var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)

// SetDecoderCallbacks per planner-time decision 10 carry-forward + ADR-0140.
// The framework's per-stream chain calls this once per stream; the filter
// stores the reference for later SendLocalReply + RequestRouteConfig +
// DownstreamPrincipal (ADR-0144; lands at Task 6) use during DecodeHeaders.
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }

// New is the HTTPFilterFactory exposed at boot. Per ADR-0072 + ADR-0140 +
// ADR-0141:
//
//  1. tc must be non-nil (an rbac filter with no typed_config surfaces a
//     config mistake; boot-time-fail-fast per ADR-0072).
//  2. Unmarshal tc to *rbacv3.RBAC; return error on malformed Any.
//  3. Invoke buildCompiledConfig with isPerRoute=false (listener-level
//     parse-time PGV-mirror checks per SPEC §6.5 + ADR-0141).
//  4. Capture *factoryState{listenerRC, reg} for per-route lazy-cache per
//     ADR-0117 + ADR-0139.
//  5. Return FilterInstanceFactory closure that allocates a fresh *filter
//     per stream bound to *factoryState. The HTTPFilter value sets Decoder:
//     f, Encoder: nil — decoder-only per §1 item 5 + ADR-0140 §Decision (iv).
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, errors.New("rbac: typed_config required")
	}
	var c rbacv3.RBAC
	if err := tc.UnmarshalTo(&c); err != nil {
		return nil, fmt.Errorf("rbac: unmarshal: %w", err)
	}
	rc, err := buildCompiledConfig(&c, ctx, false /*isPerRoute*/)
	if err != nil {
		return nil, err
	}
	state := &factoryState{
		listenerRC:    rc,
		reg:           ctx.Stats,
		hcmStatPrefix: ctx.StatPrefix,
	}
	return func() envoyhttp.HTTPFilter {
		f := &filter{state: state}
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: f,
			Encoder: nil, // decoder-only per ADR-0140 §Decision (iv)
		}
	}, nil
}

// buildCompiledConfig parses + validates an rbacv3.RBAC outer envelope per
// SPEC §6.5 + ADR-0141. The 7-consumed proto-faithful field decomposition per
// §1.1 amendment 1: rules + matcher + shadow_rules + shadow_matcher (dual-
// engine dispatch per amendment 2 — rules wins when both set; same for shadow)
// + rules_stat_prefix + shadow_rules_stat_prefix (empty allowed per amendment
// 3) + track_per_rule_stats. Outer envelope has NO silent-ignored set — the
// silent-ignore (CEL three fields + audit_logging_options) lives one level
// deeper inside config.rbac.v3.RBAC per ADR-0141.
//
// isPerRoute toggles between newFilterStats (boot-time call-site) at listener
// level and newFilterStatsIfAbsent (post-Freeze call-site) at per-route level.
// Both helpers funnel through reg.NewCounterIfAbsent — idempotent registration
// per ADR-0117 + ADR-0145 §Decision (v). The call-site distinction is
// documentation discipline only (intent: boot vs request-time).
func buildCompiledConfig(c *rbacv3.RBAC, ctx envoyhttp.FactoryCtx, isPerRoute bool) (*compiledConfig, error) {
	cc := &compiledConfig{
		rulesStatPrefix:       c.GetRulesStatPrefix(),
		shadowRulesStatPrefix: c.GetShadowRulesStatPrefix(),
		trackPerRuleStats:     c.GetTrackPerRuleStats(),
	}

	// Primary engine selection. Per §1.1 amendment 2 + rbac.pb.go:38: rules
	// wins when both rules + matcher are set. Both unset → wholly inactive
	// (per rbac.pb.go:33 "If absent, no RBAC enforcement occurs."); the filter
	// passes every request through with no counter activity.
	switch {
	case c.GetRules() != nil:
		rulesEngine, err := rbacengine.BuildRulesEngine(c.GetRules(), rbacengine.ProfileHTTP)
		if err != nil {
			return nil, err
		}
		cc.rules = rulesEngine
	case c.GetMatcher() != nil:
		matcherEngine, err := rbacengine.BuildMatcherEngine(c.GetMatcher(), rbacengine.ProfileHTTP)
		if err != nil {
			return nil, err
		}
		cc.matcher = matcherEngine
	default:
		// Neither set: filter is wholly inactive per rbac.pb.go:33.
	}

	// Shadow engine selection. Per §1.1 amendment 2: shadow_rules wins when
	// both shadow_rules + shadow_matcher are set. Both unset → no shadow walk.
	switch {
	case c.GetShadowRules() != nil:
		shadowEngine, err := rbacengine.BuildRulesEngine(c.GetShadowRules(), rbacengine.ProfileHTTP)
		if err != nil {
			return nil, err
		}
		cc.shadowRules = shadowEngine
	case c.GetShadowMatcher() != nil:
		shadowMatcherEngine, err := rbacengine.BuildMatcherEngine(c.GetShadowMatcher(), rbacengine.ProfileHTTP)
		if err != nil {
			return nil, err
		}
		cc.shadowMatcher = shadowMatcherEngine
	}

	// Stats — register the 4 base counters under the SN2-reuse namespace per
	// ADR-0145 + §1.1 amendment 9 + §11.P7. Per ADR-0085 nil-tolerance:
	// ctx.Stats may be nil in test code paths.
	//
	// Internal stat path (HCM-rooted; empirically RATIFIED at Task 8 via the
	// reference Envoy v1.37.2 stats scrape):
	//   primary: http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.{allowed,denied}
	//   shadow:  http.<HCM_stat_prefix>.rbac.<shadow_rules_stat_prefix>.{shadow_allowed,shadow_denied}
	//
	// When ctx.StatPrefix is empty (test code paths), the namespace falls back
	// to the non-HCM-rooted shape `rbac.<prefix>.<counter>` to satisfy the
	// Registry's name regex (which forbids leading dots / double dots). Per
	// ADR-0145 §Decision (vi): all 4 base counters registered unconditionally
	// (predeclared empty counters for scrape stability — operators get a
	// consistent surface regardless of engine config; matches Prometheus best
	// practice).
	if ctx.Stats != nil {
		if isPerRoute {
			cc.stats = newFilterStatsIfAbsent(ctx.Stats, ctx.StatPrefix, cc.rulesStatPrefix, cc.shadowRulesStatPrefix)
		} else {
			cc.stats = newFilterStats(ctx.Stats, ctx.StatPrefix, cc.rulesStatPrefix, cc.shadowRulesStatPrefix)
		}
	}

	return cc, nil
}

// parsePerRoute unmarshals the typed_per_filter_config Any into an
// *rbacv3.RBACPerRoute proto per §5.1 + ADR-0125 §(xii) 7th canonical
// absent-implies-disabled-OR-wholesale-override pattern. Returns the parsed
// proto for the registry's per-route map.
//
// The actual case (a)-vs-(b) disposition (disabled vs wholesale-override) is
// determined at resolvePerRouteConfig lazy-cache time via buildCompiledPerRoute
// — parsePerRoute itself does NOT validate / branch on the rbac field; it
// returns the raw proto for pointer-identity keying into factoryState.perRoute.
func parsePerRoute(tc *anypb.Any) (proto.Message, error) {
	if tc == nil {
		return nil, errors.New("rbac: per-route typed_config required")
	}
	var pr rbacv3.RBACPerRoute
	if err := tc.UnmarshalTo(&pr); err != nil {
		return nil, fmt.Errorf("rbac: per-route unmarshal: %w", err)
	}
	return &pr, nil
}

// resolvePerRouteConfig returns the *compiledConfig for the given resolved
// per-route TPFC message (returned by f.dcb.RequestRouteConfig() at
// DecodeHeaders) along with a disabled-on-this-route flag. Mirrors phase-11 +
// phase-15 lazy-cache discipline + applies CF-1 (Task 2 review I-1) signature
// disambiguation — the nil-return overloading on Task 2 was replaced with a
// triple of (rc, isDisabled, ok-for-caller-fallback):
//
//   - rc non-nil + isDisabled=false → use rc (listener-level or per-route
//     override).
//   - rc == nil + isDisabled=true → case (a) wholly disabled on this route;
//     passthrough fast-path; NO counters.
//   - rc == nil + isDisabled=false → caller falls back to listenerRC (this
//     branch should not happen in production since listenerRC is non-nil
//     after a successful New per ADR-0072).
//
// Per ADR-0117 + ADR-0125 §(xii) + ADR-0145 IMPL-1: keyed by
// *rbacv3.RBACPerRoute pointer-identity. Race-safe via sync.Map's LoadOrStore
// contract.
//
//   - msg == nil → return (s.listenerRC, false) (no per-route → inherit
//     listener).
//   - msg.(*RBACPerRoute) type-assertion failure → return (s.listenerRC,
//     false) (unexpected message type → inherit listener).
//   - cache hit → branch on cached *compiledPerRoute's disabled / overrideConfig fields directly.
//   - cache miss → buildCompiledPerRoute allocates fresh; sync.Map.LoadOrStore
//     stores or observes the winner. On per-route parse-error: log the error,
//     return (s.listenerRC, false) so the caller inherits listener — the
//     sentinel is NOT stored in the sync.Map (so a future re-resolve with the
//     same pointer-identity has another chance to succeed if the underlying
//     proto is mutated; in practice the per-route proto is read-only post-load
//     and the parse failure is sticky, but the empirical semantic is
//     log+inherit-listener-once).
func (s *factoryState) resolvePerRouteConfig(msg proto.Message) (rc *compiledConfig, isDisabled bool) {
	if msg == nil {
		return s.listenerRC, false
	}
	perRoute, ok := msg.(*rbacv3.RBACPerRoute)
	if !ok {
		return s.listenerRC, false
	}
	if cached, ok := s.perRoute.Load(perRoute); ok {
		cpr := cached.(*compiledPerRoute)
		if cpr.disabled {
			return nil, true
		}
		if cpr.overrideConfig != nil {
			return cpr.overrideConfig, false
		}
		// Cached sentinel from a previous parse-failure path: treat as
		// inherit-listener (kept for empirical-fallback determinism on
		// re-resolve).
		return s.listenerRC, false
	}
	fresh, err := buildCompiledPerRoute(perRoute, s.reg, s.hcmStatPrefix)
	if err != nil {
		// Per-route parse failed at lazy resolve. Log + fall back to
		// listener — DO NOT store the sentinel in sync.Map. Per ADR-0072
		// boot-fail-fast applies at New, not at request-time per-route
		// resolution; phase-11 / phase-15 precedent. CF-1 fix per Task 2
		// review I-1.
		log.Printf("rbac: per-route resolve failed (inherit-listener): %v", err)
		return s.listenerRC, false
	}
	actual, _ := s.perRoute.LoadOrStore(perRoute, fresh)
	cpr := actual.(*compiledPerRoute)
	if cpr.disabled {
		return nil, true
	}
	if cpr.overrideConfig != nil {
		return cpr.overrideConfig, false
	}
	return s.listenerRC, false
}

// buildCompiledPerRoute is the per-route variant of buildCompiledConfig
// invoked from resolvePerRouteConfig at lazy-cache LoadOrStore time. Per
// SPEC §6.11 + ADR-0125 §(xii) 7th canonical:
//
//   - case (a): RBACPerRoute.rbac == nil → return compiledPerRoute{disabled:
//     true, overrideConfig: nil}, nil. The route is wholly disabled
//     (passthrough; no counters).
//   - case (b): RBACPerRoute.rbac != nil → recursive buildCompiledConfig(...,
//     isPerRoute=true) → INDEPENDENT-stats *filterStats per ADR-0145.
//
// CF-1 (Task 2 review I-1) applied: returns (*compiledPerRoute, error) to
// disambiguate parse-failure from disabled-route. Mirrors phase-15
// bandwidthlimit.buildCompiledConfigPerRoute's signature
// (internal/filter/http/bandwidthlimit/bandwidthlimit.go:273). The caller
// (resolvePerRouteConfig) logs the error + falls back to listener inheritance;
// the sentinel is NOT cached in the sync.Map.
//
// Per ADR-0072 boot-fail-fast applies at New only; request-time per-route TPFC
// parse failures fall back to inherit-listener via the resolvePerRouteConfig
// path.
//
// hcmStatPrefix is threaded from factoryState.hcmStatPrefix (captured from
// FactoryCtx.StatPrefix at New time) so the per-route INDEPENDENT-stats path
// can register HCM-rooted counter names matching the listener-level shape per
// ADR-0145.
func buildCompiledPerRoute(p *rbacv3.RBACPerRoute, reg *stats.Registry, hcmStatPrefix string) (*compiledPerRoute, error) {
	if p.GetRbac() == nil {
		// Case (a): wholly disabled per §5.1 + ADR-0125 §(xii).
		return &compiledPerRoute{disabled: true, overrideConfig: nil}, nil
	}
	// Case (b): wholesale override per §5.1. Per-route INDEPENDENT-stats per
	// ADR-0145. HCM stat prefix is threaded so the per-route counter namespace
	// inherits the SAME HCM root as the listener-level counters — operators
	// see `http.<HCM>.rbac.<per_route_prefix>.<counter>` (NOT a separate
	// HCM-orphaned namespace).
	cc, err := buildCompiledConfig(p.GetRbac(), envoyhttp.FactoryCtx{Stats: reg, StatPrefix: hcmStatPrefix}, true /*isPerRoute*/)
	if err != nil {
		return nil, fmt.Errorf("rbac: per-route: %w", err)
	}
	return &compiledPerRoute{disabled: false, overrideConfig: cc}, nil
}

// newFilterStats registers the 4 base counters per ADR-0145. The 4 counters
// are registered UNCONDITIONALLY (predeclared empty counters for scrape
// stability per Prometheus best practice — operators get a consistent surface
// regardless of which engines the proto carries; shadow counters publish 0
// when no shadow engine is configured). Per-policy counters (when
// track_per_rule_stats=true) are lazy-allocated at request-time match via
// (*rbacengine.PerPolicyCounters).Inc (sync.Map LoadOrStore + NewCounterIfAbsent post-Freeze
// idempotent per ADR-0117 + ADR-0139).
//
// Internal stat path per ADR-0145 + §1.1 amendment 9 + §11.P7 (HCM-rooted
// SN2-reuse hypothesis empirically RATIFIED at Task 8 via reference Envoy
// v1.37.2 stats scrape; the scrape observed `http.<HCM>.rbac.<rules_prefix>.
// allowed` shape verbatim — NO new SN10 rule needed):
//
//	primary: http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.{allowed,denied}
//	shadow:  http.<HCM_stat_prefix>.rbac.<shadow_rules_stat_prefix>.{shadow_allowed,shadow_denied}
//
// Empty rules_stat_prefix / shadow_rules_stat_prefix is permitted per §1.1
// amendment 3; the namespacePrefix helper falls back to "rbac" so the segment
// after `rbac.` is always non-empty. Empty HCM stat_prefix (test code paths)
// folds to the non-HCM-rooted shape `rbac.<rules_prefix>.<counter>` to satisfy
// the Registry's name regex which forbids leading dots / double dots.
//
// Both helpers go through `reg.NewCounterIfAbsent` per CF-Task2-I3 + CF-Task7-M6
// — flipping from the boot-time-only `NewCounter` (which panics on duplicate)
// avoids the multi-listener-same-empty-prefix panic foot-gun. The pre-Freeze
// listener-level path (newFilterStats) and the post-Freeze per-route path
// (newFilterStatsIfAbsent) thus share the identical code path; the two
// distinct entry-points exist for documentation discipline (callsite reveals
// intent — boot-time vs request-time registration) and forward-compat with a
// possible future per-registration-mode divergence.
//
// **Caller MUST guard against nil registry**: this helper unconditionally
// dereferences reg via reg.NewCounterIfAbsent. The nil-tolerance contract
// lives at the SINGLE caller (buildCompiledConfig's `if ctx.Stats != nil`
// gate) per ADR-0085.
func newFilterStats(reg *stats.Registry, hcmPrefix, primaryPrefix, shadowPrefix string) *filterStats {
	return newFilterStatsIfAbsent(reg, hcmPrefix, primaryPrefix, shadowPrefix)
}

// newFilterStatsIfAbsent is the canonical registration entry-point per ADR-
// 0145. Both the boot-time listener-level path and the post-Freeze per-route
// path funnel here; NewCounterIfAbsent's idempotent contract per ADR-0117
// covers both registration modes. The separate newFilterStats entry-point is
// preserved at the call-site level for documentation discipline; the bodies
// are intentionally identical.
//
// Re-registration against the same prefix combination (e.g., two listener-
// level filters sharing an empty rules_stat_prefix; multi-resolve against the
// same per-route TPFC pointer where the sync.Map lazy-cache short-circuits in
// production) returns pointer-identical Counter instances per the Registry's
// byName map — race-safe via the Registry's internal mutex.
//
// **Caller MUST guard against nil registry** (ADR-0085 nil-tolerance lives at
// the buildCompiledConfig caller's `if ctx.Stats != nil` gate).
func newFilterStatsIfAbsent(reg *stats.Registry, hcmPrefix, primaryPrefix, shadowPrefix string) *filterStats {
	primaryBase := baseStatPrefix(hcmPrefix, primaryPrefix)
	shadowBase := baseStatPrefix(hcmPrefix, shadowPrefix)
	return &filterStats{
		allowed:       reg.NewCounterIfAbsent(primaryBase + ".allowed"),
		denied:        reg.NewCounterIfAbsent(primaryBase + ".denied"),
		shadowAllowed: reg.NewCounterIfAbsent(shadowBase + ".shadow_allowed"),
		shadowDenied:  reg.NewCounterIfAbsent(shadowBase + ".shadow_denied"),
		perPolicy:     &rbacengine.PerPolicyCounters{},
		reg:           reg,
		primaryBase:   primaryBase,
		shadowBase:    shadowBase,
	}
}

// namespacePrefix returns "rbac" when the configured filter-level prefix is
// empty (per §1.1 amendment 3 empty-allowed), else the configured prefix
// verbatim. The "rbac" fallback aligns with the SN2-reuse namespace shape
// where the segment between `http.<HCM>.` and `.<counter>` is always
// non-empty (Envoy v1.37.2's empirical behavior — even with empty
// rules_stat_prefix in the proto, the C++ filter substitutes `rbac` as the
// fallback segment per `utility.h::generateStats`).
func namespacePrefix(configured string) string {
	if configured == "" {
		return "rbac"
	}
	return configured
}

// baseStatPrefix returns the canonical base-prefix segment for the 4 base
// counters per ADR-0145. The shape `http.<HCM>.rbac.<rules_prefix>` is the
// HCM-rooted SN2-reuse form; empty HCM prefix folds to the bare
// `rbac.<rules_prefix>` form (test code paths; production HCM always provides
// non-empty stat_prefix at FactoryCtx.StatPrefix wiring).
//
// The Registry's nameRE forbids leading dots + double dots; the empty-HCM
// fallback is structural (NOT just a regex workaround — operators that don't
// configure HCM-rooting expect a flatter namespace anyway).
func baseStatPrefix(hcmPrefix, rulesPrefix string) string {
	rp := namespacePrefix(rulesPrefix)
	if hcmPrefix == "" {
		return "rbac." + rp
	}
	return "http." + hcmPrefix + ".rbac." + rp
}

// ---------------------------------------------------------------------------
// Dual-engine dispatch helper per SPEC §6.9 + ADR-0141. evaluateEngine
// dispatches into the shared engine's per-engine Evaluate (internal/rbac) based
// on which engine the *compiledConfig carries. The rules-engine walks policies
// in lexicographic-sorted order (first match wins; SPEC §6.9 + amendment 5);
// the matcher-engine delegates to the framework primitive (ADR-0142). Both
// concrete engine bodies + the matcher bridge moved to internal/rbac at
// phase-26.3 (D-26.3-6); the consumer keeps only the compiledConfig-shaped
// primary-vs-shadow dispatch.
// ---------------------------------------------------------------------------

// evaluateEngine walks the primary (when shadow=false) or shadow (when
// shadow=true) engine. Per SPEC §6.9 + ADR-0141 dual-engine dispatch:
//
//   - If the (primary|shadow) rules engine is set, walk it.
//   - Else if the matcher engine is set, walk it.
//   - Else defensive rbacengine.Allowed (e.g., shadow=true called when no
//     shadow engine configured — the caller's gate at SPEC §6.7 should
//     prevent this, but the helper is defensive).
//
// Returns the engine result + matched policy name (or "" if no policy matched
// OR matcher-engine no-match).
func evaluateEngine(cc *compiledConfig, ctx rbacengine.EvalContext, shadow bool) (rbacengine.EngineResult, string) {
	var rules *rbacengine.CompiledRulesEngine
	var matcherEng *rbacengine.CompiledMatcherEngine
	if shadow {
		rules = cc.shadowRules
		matcherEng = cc.shadowMatcher
	} else {
		rules = cc.rules
		matcherEng = cc.matcher
	}
	if rules != nil {
		return rules.Evaluate(ctx)
	}
	if matcherEng != nil {
		return matcherEng.Evaluate(ctx)
	}
	// Defensive: no engine configured. Per SPEC §6.9 final fall-through.
	return rbacengine.Allowed, ""
}

// ---------------------------------------------------------------------------
// *filter implements rbacengine.EvalContext per CF-6 (Task 5 notes F-1/F-2).
//
// Accessors that read per-stream state (headers, method, path) consult the
// per-stream headers map cached at DecodeHeaders time. Accessors that read
// connection-level state (DestinationIP/Port, SNI, DirectRemoteIP, RemoteIP)
// fall back to nil/empty at phase-16 MVP — the framework's connection-info
// surface is NOT yet plumbed onto DecoderFilterCallbacks per phase-16
// scope. Future framework primitives (analogous to ADR-0144's
// DownstreamPrincipal) would extend DecoderFilterCallbacks; rbac's rbacengine.EvalContext
// surface would then route through. Documented forward-pointer at
// BEHAVIOR_CONTRACT phase-16 forward-pointer notes (lands at Task 15).
//
// DownstreamPrincipal() consumes the existing ADR-0144 framework primitive
// (lands at Task 6); the *filter delegates directly to f.dcb.DownstreamPrincipal().
//
// SourcedMetadata() + FilterState() return nil at MVP (always-FALSE evaluator
// runtimes per §2.5 + §8.10).
// ---------------------------------------------------------------------------

// Compile-time assertion: *filter implements rbacengine.EvalContext.
var _ rbacengine.EvalContext = (*filter)(nil)

// Header returns the request header value for name + a presence flag. Uses
// http.Header semantics (canonical key lookup); empty-string-with-present
// distinct from absent. Pseudo-headers `:path` and `:method` are routed
// through dedicated accessors when the headers map carries them with the
// `:`-prefix (HCM dispatch typically passes :path/:method via the Go http.Header
// using canonicalized keys; production behavior is verified at fixture 0018).
func (f *filter) Header(name string) (string, bool) {
	if f.headers == nil {
		return "", false
	}
	// http.Header canonicalizes keys via CanonicalHeaderKey; CanonicalHeaderKey
	// passes `:`-prefixed names through verbatim (no canonicalization applies
	// to non-ASCII-alpha first char).
	vs, ok := f.headers[http.CanonicalHeaderKey(name)]
	if !ok || len(vs) == 0 {
		// Fall back to the raw key lookup for `:`-prefixed pseudo-headers (the
		// canonical form is the same — http.CanonicalHeaderKey on `:path`
		// returns `:path` verbatim — but defensive against caller-set Header
		// keys that bypass http.Header.Add canonicalization).
		vs, ok = f.headers[name]
		if !ok || len(vs) == 0 {
			return "", false
		}
	}
	return vs[0], true
}

// URLPath returns the `:path` H2 pseudo-header value, or "" if absent.
func (f *filter) URLPath() string {
	v, _ := f.Header(":path")
	return v
}

// Method returns the `:method` H2 pseudo-header value, or "" if absent.
func (f *filter) Method() string {
	v, _ := f.Header(":method")
	return v
}

// DestinationIP returns nil at phase-16 MVP — the framework's connection-info
// accessor is not yet exposed via DecoderFilterCallbacks. Future framework
// primitive lands; rbac's rbacengine.EvalContext widens accordingly. permDestIP evaluates
// to false until the primitive lands. Documented forward-pointer.
func (f *filter) DestinationIP() net.IP { return nil }

// DestinationPort returns 0 at phase-16 MVP — same forward-pointer as
// DestinationIP. permDestPort + permDestPortRange evaluate against 0.
func (f *filter) DestinationPort() uint32 { return 0 }

// RequestedServerName returns "" at phase-16 MVP — TLS SNI accessor not yet
// exposed via DecoderFilterCallbacks. permSNI evaluates against "" until the
// primitive lands.
func (f *filter) RequestedServerName() string { return "" }

// DirectRemoteIP returns nil at phase-16 MVP — peer-source IP accessor not
// yet exposed via DecoderFilterCallbacks. prinDirectRemoteIP evaluates to
// false until the primitive lands.
func (f *filter) DirectRemoteIP() net.IP { return nil }

// RemoteIP returns nil at phase-16 MVP — XFF-resolved IP accessor not yet
// exposed via DecoderFilterCallbacks. prinRemoteIP evaluates to false until
// the XFF resolver primitive lands per §11.P18.
func (f *filter) RemoteIP() net.IP { return nil }

// DownstreamPrincipal returns the TLS principal candidates via the ADR-0144
// framework primitive on f.dcb (lands at Task 6). nil for plaintext / non-mTLS.
func (f *filter) DownstreamPrincipal() []string {
	if f.dcb == nil {
		return nil
	}
	return f.dcb.DownstreamPrincipal()
}

// SourcedMetadata returns nil at MVP per §2.5 + §8.10 (forward-compat
// placeholder; future dynamic-metadata-family phase finalizes).
func (f *filter) SourcedMetadata() any { return nil }

// FilterState returns nil at MVP per §2.5 + §8.10 (forward-compat placeholder;
// future filter-state-family phase finalizes).
func (f *filter) FilterState() any { return nil }

// ---------------------------------------------------------------------------
// Counter emission per ADR-0145 + ADR-0146. The 4 base counters
// (allowed/denied/shadow_allowed/shadow_denied) tick unconditionally per
// disposition. Per-policy emission (when `cc.trackPerRuleStats=true` AND a
// policy matched) is wired through (*rbacengine.PerPolicyCounters).Inc — lazy
// NewCounterIfAbsent over a sync.Map per ADR-0145 §Decision (iii); the
// emission discipline (which side's base prefix; which suffix) is fixed at
// ADR-0146 §Decision (i)+(ii)+(iii).
//
// Amendment 8 (REFUTES BRAINSTORM 5-counter hypothesis): there is NO separate
// `logged` counter in Envoy v1.37.2. LOG-action engine output folds into
// engineResultAllowed (LOG always-allows per amendment 5); the allowed
// counter ticks on LOG-match. The matched-policy name IS captured for
// per-policy emission so operators get visibility into LOG-action
// per-policy ticks when track_per_rule_stats=true.
//
// Amendment 11 + §8.12: response_code_details emission is DEFERRED at
// envoy-go MVP. Envoy v1.37.2 emits `rbac_access_denied_matched_policy[
// <sanitized_policy_id>]` on DENY; envoy-go's SendLocalReply path has no
// response-code-details slot (3-arg signature). Documented divergence-
// window per ADR-0146 §Decision (iv); couples to future HCM
// response-code-details framework primitive.
// ---------------------------------------------------------------------------

// emitPrimaryCounters increments the primary-side base counter per engine
// disposition + (when trackPerRuleStats=true AND policyName != "") the
// matching per-policy counter. Nil-tolerant on cc.stats per ADR-0085.
//
// Per §1.1 amendment 8 + ADR-0146 §Decision (ii): LOG-matched paths fold
// into engineResultAllowed; the base allowed counter ticks AND (when
// track_per_rule_stats=true) the per-policy `.allowed` counter ticks. NO
// separate `logged` counter exists.
func emitPrimaryCounters(cc *compiledConfig, result rbacengine.EngineResult, policyName string) {
	if cc == nil || cc.stats == nil {
		return
	}
	var suffix string
	switch result {
	case rbacengine.Allowed:
		if cc.stats.allowed != nil {
			cc.stats.allowed.Inc()
		}
		suffix = "allowed"
	case rbacengine.Denied:
		if cc.stats.denied != nil {
			cc.stats.denied.Inc()
		}
		suffix = "denied"
	}
	// Per-policy emission gated on track_per_rule_stats AND a matched policy
	// (no-match disposition has policyName == "" — skip per-policy emission).
	// Lazy NewCounterIfAbsent via (*PerPolicyCounters).Inc per ADR-0145
	// §Decision (iii) + ADR-0146 §Decision (iii). Base prefix + policyName +
	// suffix args are byte-identical to the pre-extraction perPolicy.Inc call (R4).
	if cc.trackPerRuleStats && policyName != "" && suffix != "" {
		cc.stats.perPolicy.Inc(cc.stats.reg, cc.stats.primaryBase, policyName, suffix)
	}
}

// emitShadowCounters mirrors emitPrimaryCounters for the shadow-side base
// counters under the shadow base prefix per ADR-0146 §Decision (i). Shadow
// disposition NEVER affects primary dispatch — this helper purely emits
// counters parallel to the primary walk. Per-policy emission uses
// cc.stats.shadowBase + the shadow-suffix family (`shadow_allowed` /
// `shadow_denied`) per ADR-0146 §Decision (iii).
func emitShadowCounters(cc *compiledConfig, result rbacengine.EngineResult, policyName string) {
	if cc == nil || cc.stats == nil {
		return
	}
	var suffix string
	switch result {
	case rbacengine.Allowed:
		if cc.stats.shadowAllowed != nil {
			cc.stats.shadowAllowed.Inc()
		}
		suffix = "shadow_allowed"
	case rbacengine.Denied:
		if cc.stats.shadowDenied != nil {
			cc.stats.shadowDenied.Inc()
		}
		suffix = "shadow_denied"
	}
	if cc.trackPerRuleStats && policyName != "" && suffix != "" {
		cc.stats.perPolicy.Inc(cc.stats.reg, cc.stats.shadowBase, policyName, suffix)
	}
}

// ---------------------------------------------------------------------------
// DecodeHeaders body per SPEC §6.7. Per-route resolve → both-engines-unset
// fast-path → dual-engine dispatch → SendLocalReply on DENY.
// ---------------------------------------------------------------------------

// DecodeHeaders is the request-gate entry per ADR-0140 §Decision (iv) +
// SPEC §6.7. The body:
//
//  1. Caches the headers map on f.headers for per-stream rbacengine.EvalContext access.
//  2. Resolves per-route TPFC via f.dcb.RequestRouteConfig(); the
//     resolvePerRouteConfig accessor returns (rc, isDisabled). On
//     isDisabled=true → passthrough fast-path (no counters); rc is nil. On
//     isDisabled=false + rc non-nil → use rc as f.activeRC.
//  3. If the resolved config has BOTH primary engines unset → wholly inactive
//     per rbac.pb.go:33; passthrough fast-path (no counters).
//  4. Run primary engine: evaluateEngine(rc, ctx, shadow=false).
//  5. Run shadow engine if configured: evaluateEngine(rc, ctx, shadow=true).
//     Shadow disposition does NOT affect the dispatch; only emits counters.
//  6. Emit primary + shadow counters (STUB at Task 7 — real impl at Task 8/9).
//  7. ALLOWED → HeaderContinue.
//  8. DENIED → SendLocalReply(403, "RBAC: access denied", {Content-Type: text/plain}) + StopIteration.
//
// Per §1.1 amendment 10 + §11.P5: deny body is byte-exact "RBAC: access denied"
// (19 bytes ASCII; no trailing newline). The framework's local-reply path
// injects the remaining 3 wire headers (content-length, date, server: envoy)
// at write-time per the 4-header set discipline.
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	f.headers = headers

	// 1. Resolve per-route TPFC.
	var perRouteMsg proto.Message
	if f.dcb != nil {
		perRouteMsg = f.dcb.RequestRouteConfig()
	}
	rc, isDisabled := f.state.resolvePerRouteConfig(perRouteMsg)
	if isDisabled {
		// Case (a) wholly disabled per §5.1 + ADR-0125 §(xii). Passthrough;
		// NO counters per §11.P9.
		f.passthrough = true
		return envoyhttp.Continue
	}
	if rc == nil {
		// Defensive: resolvePerRouteConfig falls back to listenerRC which is
		// non-nil after a successful New per ADR-0072. This branch is
		// structurally unreachable in production.
		f.passthrough = true
		return envoyhttp.Continue
	}
	f.activeRC = rc

	// 2. Both engines unset → wholly inactive per rbac.pb.go:33. Passthrough;
	// NO counters.
	if rc.rules == nil && rc.matcher == nil {
		f.passthrough = true
		return envoyhttp.Continue
	}

	// 3. Primary engine evaluation. *filter is the rbacengine.EvalContext (CF-6).
	primaryResult, primaryPolicyName := evaluateEngine(rc, f, false /*shadow*/)

	// 4. Shadow engine evaluation (if configured). Shadow disposition does
	// NOT affect the dispatch outcome per §6.7 + amendment 5; only emits
	// counters.
	if rc.shadowRules != nil || rc.shadowMatcher != nil {
		shadowResult, shadowPolicyName := evaluateEngine(rc, f, true /*shadow*/)
		emitShadowCounters(rc, shadowResult, shadowPolicyName)
	}

	// 5. Emit primary counters (STUB at Task 7; real impl Task 8).
	emitPrimaryCounters(rc, primaryResult, primaryPolicyName)

	// 6. Apply disposition.
	switch primaryResult {
	case rbacengine.Allowed:
		return envoyhttp.Continue
	case rbacengine.Denied:
		// Per §1.1 amendment 10 + §11.P5: byte-exact 19-byte body; the
		// framework's local-reply path injects content-length + date + server
		// to round out the 4-header set.
		f.dcb.SendLocalReply(403, denyBody, envoyhttp.OrderedHeaders{
			{Name: "Content-Type", Value: "text/plain"},
		})
		return envoyhttp.StopIteration
	}
	// Defensive: EngineResult invariant maintained by the shared engine's
	// Evaluate methods (internal/rbac); this branch is structurally unreachable.
	return envoyhttp.Continue
}

// DecodeData is pass-through per ADR-0140 §Decision (iv) — RBAC evaluates at
// DecodeHeaders before body bytes flow. No per-stream body interaction.
func (f *filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// DecodeTrailers is pass-through per ADR-0140 §Decision (iv).
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// OnDestroy is a no-op per ADR-0140 §Decision (iv) — RBAC has no async
// resources to release.
func (f *filter) OnDestroy() {}
