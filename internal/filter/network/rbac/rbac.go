// Package rbac is the rbac_network L4 network filter — consumer #2 of the
// shared internal/rbac engine (consumer #1 is internal/filter/http/rbac). It
// parses an envoy.extensions.filters.network.rbac.v3.RBAC typed_config at boot,
// compiles the enforced + shadow engines under the L4 input-capability profile
// (ProfileL4 — HTTP-only permission/principal arms PARSE-REJECT at compile),
// and predeclares the four `<stat_prefix>.rbac.*` counters for scrape stability.
//
// OnNewConnection is a sticky-halt-safe Continue no-op. OnData walks the
// enforced (+ shadow-counter) engines and applies the disposition (allow →
// Continue / enforced-deny → rcd + NoFlush close). The shadow walk
// additionally writes the shadow pair (shadow_engine_result +
// shadow_effective_policy_id) to the per-connection dynamic-metadata bucket —
// the first per-connection production consumer of the connection-scoped
// *dynamicmetadata.Bucket (ADR-0217).
package rbac

import (
	"errors"
	"fmt"

	matchv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	configrbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	networkrbacv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/rbac/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pgdad/envoy-go/internal/filter/network"
	rbacengine "github.com/pgdad/envoy-go/internal/rbac"
	"github.com/pgdad/envoy-go/internal/stats"
)

// TypeURL is the canonical Any type URL for the rbac_network typed_config. It is
// DERIVED via proto.MessageName (NOT hand-typed): the network-filter type URLs
// carry an `extensions.` segment that bit a prior phase (memory
// reference_network_filter_typeurl_extensions), so the only safe source is the
// generated descriptor.
var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&networkrbacv3.RBAC{}))

// PARSE-REJECT wording constants. These exact strings are pinned by the
// byte-stable tests; do not change the wording.
const (
	parseRejectStatPrefixRequired      = "rbac_network: stat_prefix is required"
	parseRejectStatPrefixInvalid       = "rbac_network: stat_prefix contains characters invalid for a metric name"
	parseRejectShadowStatPrefixInvalid = "rbac_network: shadow_rules_stat_prefix contains characters invalid for a metric name"
	parseRejectDelayDeny               = "rbac_network: delay_deny is unsupported"
)

// filterStats is the four predeclared base counters for one rbac_network filter
// instance. NO PerPolicyCounters (F2 — the network consumer passes
// trackPerRuleStats=false implicitly; the per-policy cache is never constructed).
type filterStats struct {
	allowed       *stats.Counter // enforced engine result = ALLOWED
	denied        *stats.Counter // enforced engine result = DENIED
	shadowAllowed *stats.Counter // shadow engine result = ALLOWED (publishes 0 when no shadow)
	shadowDenied  *stats.Counter // shadow engine result = DENIED
}

// compiledConfig is the boot-resolved runtime config shared by every
// per-connection filter instance. The enforced + shadow engines are mutually
// exclusive within each pair (rules wins when both set, mirroring the HTTP
// consumer + §1.1 amendment 2).
type compiledConfig struct {
	// Invariant: at most one of (enforcedRules, enforcedMatcher) is non-nil, and at most one of (shadowRules, shadowMatcher) is non-nil — enforced by buildEngines; OnData dispatch relies on this.
	enforcedRules   *rbacengine.CompiledRulesEngine   // nil when matcher set (or wholly inactive)
	enforcedMatcher *rbacengine.CompiledMatcherEngine // nil when rules set or wholly inactive
	shadowRules     *rbacengine.CompiledRulesEngine
	shadowMatcher   *rbacengine.CompiledMatcherEngine

	// enforcementContinuous mirrors enforcement_type == CONTINUOUS. OnData
	// consults this to decide one-time-on-first-byte vs continuous
	// re-evaluation.
	enforcementContinuous bool

	stats *filterStats
}

// NewFactory returns the rbac_network NetworkFilterFactory with the stats
// Registry closure-captured (D-26.3-3: the network FactoryCtx carries no stats
// registry; the boot-singleton is captured at registration time, mirroring
// tcpproxy.NewNetworkFactory / hcm.NewNetworkFactory). reg may be nil in test
// code paths; the factory guards against it (ADR-0085 nil-tolerance).
func NewFactory(reg *stats.Registry) network.NetworkFilterFactory {
	return func(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
		if tc == nil {
			return nil, errors.New("rbac_network: typed_config required")
		}
		var c networkrbacv3.RBAC
		if err := tc.UnmarshalTo(&c); err != nil {
			return nil, fmt.Errorf("rbac_network: unmarshal: %w", err)
		}

		// PGV-required stat_prefix (the four counters root on it).
		if c.GetStatPrefix() == "" {
			return nil, errors.New(parseRejectStatPrefixRequired)
		}
		// Validate stat_prefix at the input boundary before passing it to
		// newFilterStats, which calls NewCounterIfAbsent (panics on invalid names).
		// Probe the full metric name that would be registered — a digit-leading or
		// otherwise invalid prefix would pass the != "" check but panic later.
		// Mirrors hcm/config.go:221 and lua/compiled_config.go:365. (ADR-0059)
		//
		// Probe the assembled counter name (suffix must match newFilterStats:
		// enforcedBase = statPrefix+".rbac", counter = enforcedBase+".allowed").
		// nameRE is whole-string-anchored so a bare-prefix check would mis-accept.
		if !stats.IsValidName(c.GetStatPrefix() + ".rbac.allowed") {
			return nil, errors.New(parseRejectStatPrefixInvalid)
		}
		// Validate shadow_rules_stat_prefix when present (it forms
		// statPrefix+".rbac."+shadowRulesStatPrefix+".shadow_allowed").
		//
		// Probe the assembled shadow counter name (suffix must match newFilterStats:
		// shadowBase = enforcedBase+"."+shadowRulesStatPrefix, counter = shadowBase+".shadow_allowed").
		if sp := c.GetShadowRulesStatPrefix(); sp != "" {
			if !stats.IsValidName(c.GetStatPrefix() + ".rbac." + sp + ".shadow_allowed") {
				return nil, errors.New(parseRejectShadowStatPrefixInvalid)
			}
		}
		// delay_deny PARSE-REJECT (AMEND-A9): the deferred-deny timer surface is
		// unsupported at envoy-go MVP.
		if c.GetDelayDeny() != nil {
			return nil, errors.New(parseRejectDelayDeny)
		}

		cc := &compiledConfig{
			enforcementContinuous: c.GetEnforcementType() == networkrbacv3.RBAC_CONTINUOUS,
		}

		// Enforced engine selection. Rules wins when both set (mirrors the HTTP
		// consumer's buildCompiledConfig dispatch + §1.1 amendment 2). Both unset
		// → wholly inactive. ProfileL4 PARSE-REJECTs HTTP-only arms at compile.
		rules, matcher, err := buildEngines(c.GetRules(), c.GetMatcher())
		if err != nil {
			return nil, err
		}
		cc.enforcedRules, cc.enforcedMatcher = rules, matcher

		// Shadow engine selection (shadow_rules wins when both set).
		shadowRules, shadowMatcher, err := buildEngines(c.GetShadowRules(), c.GetShadowMatcher())
		if err != nil {
			return nil, err
		}
		cc.shadowRules, cc.shadowMatcher = shadowRules, shadowMatcher

		// Predeclare the four counters at PARSE (register-at-parse) so they exist
		// at scrape even before any traffic. Guarded against a nil registry per
		// ADR-0085.
		if reg != nil {
			cc.stats = newFilterStats(reg, c.GetStatPrefix(), c.GetShadowRulesStatPrefix())
		}

		return func() network.NetworkFilter { return &filter{cc: cc} }, nil
	}
}

// buildEngines applies the rules-wins-if-both dispatch under ProfileL4 for one
// (rules, matcher) pair. Mirrors the HTTP consumer's switch; both nil → wholly
// inactive (both returns nil, nil, nil).
func buildEngines(rules *configrbacv3.RBAC, matcher *matchv3.Matcher) (*rbacengine.CompiledRulesEngine, *rbacengine.CompiledMatcherEngine, error) {
	switch {
	case rules != nil:
		re, err := rbacengine.BuildRulesEngine(rules, rbacengine.ProfileL4)
		if err != nil {
			return nil, nil, err
		}
		return re, nil, nil
	case matcher != nil:
		me, err := rbacengine.BuildMatcherEngine(matcher, rbacengine.ProfileL4)
		if err != nil {
			return nil, nil, err
		}
		return nil, me, nil
	default:
		return nil, nil, nil
	}
}

// newFilterStats predeclares the four base counters under the chain-scoped
// `<stat_prefix>.rbac.*` namespace per SPEC §7. stat_prefix is the namespace
// ROOT (network shape; NOT the HCM-rooted `http.<HCM>.rbac.*` HTTP shape). The
// shadow_rules_stat_prefix inserts a segment between `rbac.` and the two
// shadow_* counters ONLY (SPEC §7.1) — the enforced counters are unaffected.
//
// All four are registered unconditionally (predeclared-empty for scrape
// stability, Prometheus best practice) via NewCounterIfAbsent (idempotent — two
// chains sharing a stat_prefix return pointer-identical counters).
func newFilterStats(reg *stats.Registry, statPrefix, shadowRulesStatPrefix string) *filterStats {
	enforcedBase := statPrefix + ".rbac"
	shadowBase := enforcedBase
	if shadowRulesStatPrefix != "" {
		shadowBase = enforcedBase + "." + shadowRulesStatPrefix
	}
	return &filterStats{
		allowed:       reg.NewCounterIfAbsent(enforcedBase + ".allowed"),
		denied:        reg.NewCounterIfAbsent(enforcedBase + ".denied"),
		shadowAllowed: reg.NewCounterIfAbsent(shadowBase + ".shadow_allowed"),
		shadowDenied:  reg.NewCounterIfAbsent(shadowBase + ".shadow_denied"),
	}
}

// filterName is the rbac_network canonical name — used as the dynamic-metadata
// NAMESPACE for the shadow-pair writes (the first per-connection production
// consumer of the connection-scoped *dynamicmetadata.Bucket; ADR-0217). Stats
// already root on the literal stat_prefix; this const is the metadata namespace.
// Byte-faithful to upstream's `envoy.filters.network.rbac`.
const filterName = "envoy.filters.network.rbac"

// rbacDenyCloseRCD is the response-code-details string set on an enforced deny
// before the NoFlush close. It matches upstream's
// setConnectionTerminationDetails("rbac_deny_close"). Byte-stable: do not
// change this string.
const rbacDenyCloseRCD = "rbac_deny_close"

// evalEnforced walks the enforced engine (rules wins when both set, mirroring
// the HTTP consumer's evaluateEngine). When neither enforced engine is set the
// filter is wholly inactive (rbac.pb.go:33 "If absent, no RBAC enforcement
// occurs.") — the HTTP consumer treats this as passthrough (Continue, NO
// counters) via the BOTH-nil gate in DecodeHeaders, and evaluateEngine itself
// returns a defensive Allowed. OnData reproduces that gate BEFORE calling this
// helper, so the (nil,nil) → Allowed fall-through here is defensive only.
func (cc *compiledConfig) evalEnforced(ctx rbacengine.EvalContext) (rbacengine.EngineResult, string) {
	if cc.enforcedRules != nil {
		return cc.enforcedRules.Evaluate(ctx)
	}
	if cc.enforcedMatcher != nil {
		return cc.enforcedMatcher.Evaluate(ctx)
	}
	return rbacengine.Allowed, ""
}

// evalShadow walks the shadow engine (shadow_rules wins when both set). Only
// reached when at least one shadow engine is configured (the OnData caller
// gates on shadowRules != nil || shadowMatcher != nil); the final fall-through
// is defensive, mirroring the HTTP consumer's evaluateEngine.
func (cc *compiledConfig) evalShadow(ctx rbacengine.EvalContext) (rbacengine.EngineResult, string) {
	if cc.shadowRules != nil {
		return cc.shadowRules.Evaluate(ctx)
	}
	if cc.shadowMatcher != nil {
		return cc.shadowMatcher.Evaluate(ctx)
	}
	return rbacengine.Allowed, ""
}

// emitShadow ticks the shadow_allowed / shadow_denied counter for a shadow-walk
// result. Guarded against a nil stats block (ADR-0085: NewFactory(nil) leaves
// cc.stats nil in test code paths). The shadow metadata pair
// (shadow_engine_result + shadow_effective_policy_id) is written in OnData's
// shadow walk, not here.
func (cc *compiledConfig) emitShadow(res rbacengine.EngineResult) {
	if cc.stats == nil {
		return
	}
	switch res {
	case rbacengine.Allowed:
		cc.stats.shadowAllowed.Inc()
	default: // rbacengine.Denied
		cc.stats.shadowDenied.Inc()
	}
}

// filter is the per-connection rbac_network read-filter instance. The
// enforcement decision runs in OnData (AMEND-A8): OnNewConnection MUST return
// Continue — a StopIteration there sets the chain's sticky connHalted flag and
// blocks ALL subsequent OnData calls (memory
// reference_network_read_filter_onnewconnection_halts), so the decision would
// never run. The decided flag drives ONE_TIME_ON_FIRST_BYTE: once a disposition
// is reached, subsequent OnData calls pass through (unless enforcement_type ==
// CONTINUOUS, which re-decides every OnData).
type filter struct {
	network.Marker
	cc      *compiledConfig
	cb      network.ReadFilterCallbacks
	decided bool // ONE_TIME guard: true once a disposition has been reached
}

// OnNewConnection returns Continue. The enforcement decision is deferred to
// OnData (AMEND-A8): a StopIteration here would set the chain's sticky
// connHalted flag and block ALL subsequent OnData calls (memory
// reference_network_read_filter_onnewconnection_halts).
func (f *filter) OnNewConnection() network.Status { return network.Continue }

// OnData runs the enforcement decision (AMEND-A8). ONE_TIME_ON_FIRST_BYTE (the
// default enforcement_type) decides once — subsequent OnData calls pass through
// via the decided guard; CONTINUOUS (enforcementContinuous) re-decides every
// call. A missing enforced engine means the filter is wholly inactive
// (rbac.pb.go:33) → passthrough Continue, NO counters (mirrors the HTTP
// consumer's BOTH-nil gate). Allow → Continue (advance to the terminal /
// prefixConn handover). Enforced deny → response-code-details "rbac_deny_close"
// + Connection().Close(NoFlush) + StopIteration (AMEND-A7).
func (f *filter) OnData(_ *network.Buffer, _ bool) network.Status {
	// ONE_TIME: a disposition was already reached → pass through. CONTINUOUS
	// re-decides every OnData (enforcementContinuous bypasses the guard).
	if f.decided && !f.cc.enforcementContinuous {
		return network.Continue
	}

	// Wholly inactive (neither enforced engine set) → passthrough, NO counters
	// (rbac.pb.go:33; mirrors the HTTP consumer's BOTH-nil gate in
	// DecodeHeaders). The decided flag is NOT set: an inactive filter has no
	// disposition to memoize.
	if f.cc.enforcedRules == nil && f.cc.enforcedMatcher == nil {
		return network.Continue
	}

	f.decided = true
	ctx := newL4EvalContext(f.cb.Connection())

	// Shadow walk (never affects the enforced disposition): ticks the
	// shadow_allowed / shadow_denied counter AND writes the shadow pair
	// (shadow_engine_result + shadow_effective_policy_id) to the per-connection
	// dynamic-metadata bucket (ADR-0217 — the FIRST per-connection production
	// WRITE through *dynamicmetadata.Bucket; REUSE of the connection-scoped
	// bucket, no internal/dynamicmetadata/ code change per R5). Keys + result
	// strings are byte-faithful to upstream. The bucket is nil-receiver tolerant
	// (ADR-0085), so no nil-guard on DynamicMetadata() is required.
	if f.cc.shadowRules != nil || f.cc.shadowMatcher != nil {
		sres, sname := f.cc.evalShadow(ctx)
		f.cc.emitShadow(sres) // shadow_allowed / shadow_denied
		bucket := f.cb.DynamicMetadata()
		result := "allowed"
		if sres == rbacengine.Denied {
			result = "denied"
		}
		bucket.Set(filterName, "shadow_engine_result", structpb.NewStringValue(result))
		if sname != "" { // upstream parity: write the id only when non-empty
			bucket.Set(filterName, "shadow_effective_policy_id", structpb.NewStringValue(sname))
		}
	}

	// Enforced walk.
	res, _ := f.cc.evalEnforced(ctx)
	switch res {
	case rbacengine.Allowed:
		if f.cc.stats != nil {
			f.cc.stats.allowed.Inc()
		}
		return network.Continue // advance to the terminal (prefixConn handover)
	default: // rbacengine.Denied
		if f.cc.stats != nil {
			f.cc.stats.denied.Inc()
		}
		// SetResponseCodeDetails is an optional-interface sink on the concrete
		// callbacks (the chain's *callbacks, per the TestResponseCodeDetailsSink
		// + directresponse's pattern), not a method on the ReadFilterCallbacks
		// interface.
		if s, ok := f.cb.(interface{ SetResponseCodeDetails(string) }); ok {
			s.SetResponseCodeDetails(rbacDenyCloseRCD)
		}
		f.cb.Connection().Close(network.NoFlush)
		return network.StopIteration
	}
}

// SetReadFilterCallbacks stores the per-connection callbacks handle.
func (f *filter) SetReadFilterCallbacks(cb network.ReadFilterCallbacks) { f.cb = cb }

// OnDestroy is a no-op; the filter holds no resources that require explicit cleanup.
func (f *filter) OnDestroy() {}
