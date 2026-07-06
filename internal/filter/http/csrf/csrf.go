package csrf

import (
	"errors"
	"net/http"
	"net/url"

	csrfv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
)

// TypeURL is the canonical envoy.filters.http.csrf typed_config type URL.
// Boot wiring in cmd/envoy-go/main.go registers New under this key in the
// HTTPRegistry per ADR-0072.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.csrf.v3.CsrfPolicy"

// filterName is the canonical http_filters[].name string for csrf
// (matches the listener config typed_per_filter_config map keys).
const filterName = "envoy.filters.http.csrf"

// New is the HTTPFilterFactory exposed at boot. Per SPEC §11.11 amendment the
// CsrfPolicy.filter_enabled field is REQUIRED at parse-time — envoy-go
// PGV-mirrors Envoy's behavior by rejecting nil filter_enabled or nil
// filter_enabled.default_value with a non-nil error.
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, errors.New("csrf: typed_config required")
	}
	var c csrfv3.CsrfPolicy
	if err := tc.UnmarshalTo(&c); err != nil {
		return nil, err
	}
	// Construct the *filterStats; nil-tolerance for test code paths per ADR-0085
	// (callers in production wiring always provide a non-nil Registry, mirroring
	// phase 11's local_ratelimit call-site guard pattern).
	var listenerStats *filterStats
	if ctx.Stats != nil {
		listenerStats = newFilterStats(ctx.Stats, ctx.StatPrefix)
	}
	rc, err := buildRuntimeConfig(&c, listenerStats)
	if err != nil {
		return nil, err
	}
	return func() envoyhttp.HTTPFilter {
		f := &filter{rc: rc}
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: f,
			Encoder: nil, // decoder-only per planner-time decision 2
		}
	}, nil
}

// runtimeConfig captures the 2 runtime fields per SPEC §6.2 (the proto's 3
// top-level fields decompose to: 1 consumed → additionalOrigins; 1
// PGV-validated-not-honored → filter_enabled (silent-ignored at runtime);
// 1 deferred → shadow_enabled). The stats pointer is the listener-level
// pointer; per-route runtimeConfig SHARES this pointer per §11.9 amendment.
type runtimeConfig struct {
	additionalOrigins []string     // verbatim host[:port] entries from surviving exact-with-non-empty StringMatcher
	stats             *filterStats // listener-level; per-route SHARES this
}

// filterStats is the 3-counter set per SPEC §6.6 + ADR-0124. Counters are
// *stats.Counter (lock-free atomic Inc per ADR-0061); allocated once at New
// time, shared across all goroutines processing requests through this filter
// instance (and across listener-level + per-route runtimeConfigs per §11.9).
type filterStats struct {
	requestValid        *stats.Counter
	requestInvalid      *stats.Counter
	missingSourceOrigin *stats.Counter
}

// filter is the per-instance per-stream filter state. Per ADR-0071's single-
// goroutine-per-stream invariant, the per-instance state is race-free without
// synchronization. The rc pointer is closure-captured at New time and is
// immutable post-construction.
type filter struct {
	rc  *runtimeConfig
	dcb envoyhttp.DecoderFilterCallbacks
}

// Statically assert decoder-only conformance per planner-time decision 1.
var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)

func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }
func (f *filter) OnDestroy()                                              {}

// DecodeData / DecodeTrailers are pass-through (csrf settles disposition in
// DecodeHeaders).
func (f *filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// modifyingMethods is the canonical 4-method set per §11.1 empirical pin.
// Case-sensitive uppercase string match against the :method pseudo-header.
var modifyingMethods = map[string]struct{}{
	"POST":   {},
	"PUT":    {},
	"DELETE": {},
	"PATCH":  {},
}

// DecodeHeaders implements the disposition table per SPEC §6.5 + ADR-0122 + ADR-0123.
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	method := headers.Get(":method")
	if _, ok := modifyingMethods[method]; !ok {
		return envoyhttp.Continue // non-modifying: short-circuit BEFORE any state touch
	}

	// Resolve the effective runtimeConfig: per-route override OR listener-level fallback.
	rc := f.rc
	if perRoute := f.dcb.RequestRouteConfig(); perRoute != nil {
		if c, ok := perRoute.(*csrfv3.CsrfPolicy); ok {
			pr, err := buildPerRouteRuntime(c, f.rc.stats)
			if err == nil && pr != nil {
				rc = pr
			}
			// On err: fall through to listener-level rc. (Per-route invalid configs
			// are caught by reference Envoy at PGV; differential equivalence is
			// preserved for well-formed configs. See planner-time decision 5.)
		}
	}

	target := targetOriginValue(headers)
	source := sourceOriginValue(headers)
	allow, missing := evaluate(rc, source, target)
	switch {
	case allow:
		rc.stats.requestValid.Inc()
		return envoyhttp.Continue
	case missing:
		rc.stats.missingSourceOrigin.Inc()
	default:
		rc.stats.requestInvalid.Inc()
	}
	f.dcb.SendLocalReply(403, "Invalid origin", envoyhttp.OrderedHeaders{
		{Name: "Content-Type", Value: "text/plain"},
	})
	return envoyhttp.StopIteration
}

// sourceOriginValue implements the §11.2 trichotomy.
//  1. Origin: "null" literal → empty (NO Referer fallback).
//  2. Origin empty/absent OR yields empty hostAndPort → fall back to Referer's hostAndPort.
//  3. Origin non-empty, non-"null", URL parse fails → return verbatim (NO Referer fallback).
func sourceOriginValue(headers http.Header) string {
	originVal := headers.Get("Origin")
	if originVal == "null" {
		return "" // case 1: missing_source_origin path
	}
	if originVal != "" {
		// Case 3 OR successful parse: hostAndPort returns either u.Host or
		// the verbatim string on parse failure.
		hp := hostAndPort(originVal)
		if hp != "" {
			return hp
		}
		// hp == "" only if originVal was "" (already filtered above) — defensive.
	}
	// Case 2: Origin empty or absent → fall back to Referer.
	refererVal := headers.Get("Referer")
	if refererVal == "" {
		return ""
	}
	return hostAndPort(refererVal)
}

// targetOriginValue constructs the target host:port from the request's Host
// or :authority header. Per planner-time decision 8 + §11.3 amendment: scheme
// is computed only to make the URL parser accept the input then stripped via
// hostAndPort(); we use a synthetic "http://" prefix.
func targetOriginValue(headers http.Header) string {
	host := headers.Get(":authority")
	if host == "" {
		host = headers.Get("Host")
	}
	if host == "" {
		return ""
	}
	return hostAndPort("http://" + host)
}

// hostAndPort parses an absolute URL and returns the host[:port] portion.
// On parse failure (or empty u.Host), returns the verbatim input — mirrors
// Envoy's Http::Utility::Url::initialize fallback per §11.2 source-of-truth.
func hostAndPort(absoluteURL string) string {
	if absoluteURL == "" {
		return ""
	}
	u, err := url.Parse(absoluteURL)
	if err != nil || u.Host == "" {
		return absoluteURL
	}
	return u.Host
}

// evaluate implements the disposition algorithm per §6.4. Empty source →
// missing path. Non-empty source: check additionalOrigins[] first (any byte-
// equal match → allow), then target equality (byte-equal → allow), else reject.
func evaluate(rc *runtimeConfig, source, target string) (allow bool, missing bool) {
	if source == "" {
		return false, true // missing_source_origin
	}
	for _, additional := range rc.additionalOrigins {
		if source == additional {
			return true, false
		}
	}
	if source == target {
		return true, false
	}
	return false, false
}

// buildPerRouteRuntime is the per-route runtimeConfig builder (per §11.9 +
// planner-time decision 5). The per-route runtimeConfig SHARES the listener-
// level *filterStats pointer; only the additionalOrigins slice is independent.
// PGV-mirror validation runs the same as listener-level (filter_enabled
// non-nil + non-nil DefaultValue).
func buildPerRouteRuntime(perRoute *csrfv3.CsrfPolicy, listenerStats *filterStats) (*runtimeConfig, error) {
	return buildRuntimeConfig(perRoute, listenerStats)
}

// buildRuntimeConfig validates the proto + compiles additional_origins. The
// listenerStats pointer is shared into the resulting *runtimeConfig per §11.9
// (per-route runtimeConfig will re-use this pointer via buildPerRouteRuntime
// per planner-time decision 5).
func buildRuntimeConfig(c *csrfv3.CsrfPolicy, listenerStats *filterStats) (*runtimeConfig, error) {
	if c.GetFilterEnabled() == nil {
		return nil, errors.New("csrf: filter_enabled is required")
	}
	if c.GetFilterEnabled().GetDefaultValue() == nil {
		return nil, errors.New("csrf: filter_enabled.default_value is required")
	}
	// shadow_enabled is OPTIONAL — no validation per §11.11 probe #3.
	// filter_enabled.default_value's percentage value is silent-ignored at runtime per §1.1 amendment 3 — we do NOT inspect numerator/denominator.

	compiled := compileAdditionalOrigins(c.GetAdditionalOrigins())
	return &runtimeConfig{
		additionalOrigins: compiled,
		stats:             listenerStats,
	}, nil
}

// compileAdditionalOrigins applies the ADR-0101 §3 parse-time-drop discipline:
// only StringMatcher.exact variant with non-empty value is honored; all other
// variants (prefix, suffix, contains, safe_regex, ignore_case-with-anything)
// are dropped at PARSE time. Surviving entries are stored verbatim — NO
// normalization (NO case folding, NO default-port stripping) per §11.7 amendment.
func compileAdditionalOrigins(matchers []*matcherv3.StringMatcher) []string {
	out := make([]string, 0, len(matchers))
	for _, m := range matchers {
		if m == nil {
			continue
		}
		if m.GetIgnoreCase() {
			// ignore_case is a non-exact MODIFIER even on top of an exact
			// pattern; drop per ADR-0101 §3 ("only HeaderMatcher_StringMatch
			// with non-empty Exact value is honored").
			continue
		}
		exact, ok := m.GetMatchPattern().(*matcherv3.StringMatcher_Exact)
		if !ok {
			continue
		}
		if exact.Exact == "" {
			continue
		}
		out = append(out, exact.Exact)
	}
	return out
}

// newFilterStats registers the 3-counter set at the HCM-level stat_prefix per
// SPEC §6.6. Stats anchor at "http.<hcmStatPrefix>.csrf.<counter>" per the
// existing SN2 rule from ADR-0061; NO new SN flattening rule needed (UNLIKE
// phase 11 which introduced SN9 for filter-specific tag-extraction). Caller
// MUST guarantee `reg != nil` — the call site in `New` guards via
// `ctx.Stats != nil` per ADR-0085 nil-tolerance pattern (mirrors phase 11
// local_ratelimit's call-site-guard precedent at local_ratelimit.go:204-207).
func newFilterStats(reg *stats.Registry, hcmStatPrefix string) *filterStats {
	prefix := "http." + hcmStatPrefix + ".csrf."
	return &filterStats{
		requestValid:        reg.NewCounter(prefix + "request_valid"),
		requestInvalid:      reg.NewCounter(prefix + "request_invalid"),
		missingSourceOrigin: reg.NewCounter(prefix + "missing_source_origin"),
	}
}
