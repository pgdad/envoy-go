package localratelimit

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// TypeURL is the canonical envoy.filters.http.local_ratelimit typed_config type URL.
// Boot wiring in cmd/envoy-go/main.go (Task 7) registers New under this key
// in the HTTPRegistry per ADR-0072.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit"

// filterName is the canonical http_filters[].name string for local_ratelimit
// (matches the listener config typed_per_filter_config map keys).
const filterName = "envoy.filters.http.local_ratelimit"

// rateLimitedBody is the canonical 18-byte body emitted on the rate-limited
// response per SPEC §11.3 empirical pin (ASCII; NO trailing newline; MD5
// 397e830923f3080ba63b3d38b53678ac). Per ADR-0119.
// Const-string form matches the fault precedent (faultAbortBody at
// internal/filter/http/fault/fault.go:27) and the SendLocalReply(string)
// signature — no []byte conversion needed.
const rateLimitedBody = "local_rate_limited"

// minFillInterval is the filter-internal minimum on token_bucket.fill_interval
// per SPEC §11.2c empirical pin. The check fires AFTER proto unmarshal succeeds,
// NOT as a PGV constraint; the verbatim error string at New time mirrors
// Envoy v1.37.2's source/server/config_validation/server.cc:76 message exactly.
// Per ADR-0115.
const minFillInterval = 50 * time.Millisecond

// runtimeConfig is the per-instance parsed config shape per ADR-0115.
//
// Five fields consumed at request-eval time (statPrefix, bucket, statusCode,
// body, stats); fourteen LocalRateLimit fields silently ignored per SPEC §2.1
// + ADR-0040 silent-ignore discipline (full list at BEHAVIOR_CONTRACT §13.1).
type runtimeConfig struct {
	statPrefix string       // from cfg.StatPrefix (PGV non-empty per §11.1)
	bucket     *tokenBucket // closure-captured per filter-instance / per per-route entry
	statusCode int          // from cfg.Status.Code (default 429 per §11.4; PGV [400, 600))
	body       string       // literal "local_rate_limited" (18 bytes; per §11.3 + ADR-0119)
	stats      *filterStats // 4 counters scoped by stat_prefix; nil when ctx.Stats is nil (test path)
}

// filterStats is the 4-counter set per ADR-0118 + SPEC §11.5.
//
// All four counters are *stats.Counter (lock-free counter increments per ADR-0061);
// the Inc-side is mutex-free. The four-counter set is the COMPLETE surface (per
// SPEC §11.5 conclusion (a): no near_limit, no total_pending, no dynamic-metadata
// gauges).
type filterStats struct {
	enabled     *stats.Counter // every req reaching the filter
	ok          *stats.Counter // tryConsume → true
	rateLimited *stats.Counter // tryConsume → false
	enforced    *stats.Counter // tryConsume → false; lockstep with rateLimited per MVP
}

// factoryState is the closure-captured shared state per factory invocation.
// listenerRC is the listener-level *runtimeConfig (immutable post-construction).
// perRoute is a lazy-cache keyed by *localratelimitv3.LocalRateLimit pointer;
// populated on first resolve via resolvePerRouteConfig. Per ADR-0117.
//
// IMPL-1 (PROGRESS.md preamble): upstream Envoy v1.37.2 reuses the same
// LocalRateLimit proto for both listener-level and per-route TPFC entries;
// there is no separate LocalRateLimitPerRoute message. The lazy-cache key is
// therefore *localratelimitv3.LocalRateLimit (pointer identity = distinct per-route entry).
type factoryState struct {
	listenerRC *runtimeConfig
	perRoute   sync.Map // map[*localratelimitv3.LocalRateLimit]*runtimeConfig — IMPL-1: LocalRateLimit (not LocalRateLimitPerRoute) per PROGRESS.md preamble
	reg        *stats.Registry
}

// filter is the per-request filter instance allocated by the factory closure.
// State is request-scoped; *factoryState is the closure-captured shared state
// (immutable post-construction except for the sync.Map lazy-cache; race-safe
// per sync.Map's contract).
type filter struct {
	state *factoryState
	dcb   envoyhttp.DecoderFilterCallbacks
	ecb   envoyhttp.EncoderFilterCallbacks // unused per SPEC §6.5 NOTE; kept per planner-time decision 2 for chain-of-conformance
}

// Statically assert the both-sides interface conformance (matches cors precedent).
var (
	_ envoyhttp.StreamDecoderFilter = (*filter)(nil)
	_ envoyhttp.StreamEncoderFilter = (*filter)(nil)
)

// SetDecoderCallbacks / SetEncoderCallbacks per planner-time decision 2.
// The framework's per-stream chain (internal/filter/http/chain.go) calls
// SetDecoderCallbacks once per stream; the filter stores the reference for
// later use during DecodeHeaders.
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }

// New is the HTTPFilterFactory exposed at boot. Per ADR-0114 + ADR-0115:
//
//  1. tc must be non-nil (a local_ratelimit filter with no typed_config has
//     no behavioral effect; surface configuration mistakes at boot per
//     ADR-0072 boot-time-fail-fast).
//  2. Unmarshal tc to *localratelimitv3.LocalRateLimit; return error on
//     malformed Any.
//  3. Run six explicit validation checks (per planner-time decision 3):
//     stat_prefix non-empty, max_tokens > 0, tokens_per_fill > 0 if explicit
//     [defaults to 1 if absent], fill_interval >= 50ms [filter-internal not
//     PGV; verbatim Envoy error string], status.code in [400, 600) if explicit
//     [defaults to 429 if absent].
//  4. Construct *tokenBucket via newTokenBucket(maxTokens, tokensPerFill, fillInterval).
//  5. Construct *filterStats via newFilterStats(ctx.Stats, statPrefix).
//  6. Construct *runtimeConfig capturing the 5 consumed fields.
//  7. Capture factoryState{listenerRC, reg} for per-route lazy-cache (ADR-0117).
//  8. Return FilterInstanceFactory closure that allocates a fresh *filter
//     per request bound to *factoryState.
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, errors.New("local_ratelimit: typed_config required")
	}
	var c localratelimitv3.LocalRateLimit
	if err := tc.UnmarshalTo(&c); err != nil {
		return nil, fmt.Errorf("local_ratelimit: unmarshal: %w", err)
	}
	rc, err := buildRuntimeConfig(&c, ctx)
	if err != nil {
		return nil, err
	}
	state := &factoryState{
		listenerRC: rc,
		reg:        ctx.Stats,
	}
	return func() envoyhttp.HTTPFilter {
		f := &filter{state: state}
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: f,
			Encoder: f,
		}
	}, nil
}

// buildRuntimeConfig parses + validates the 5 consumed fields from the
// LocalRateLimit proto. Per planner-time decision 3: six explicit checks.
// Per ADR-0115.
//
// The bucket + stats fields are wired here (Task 2+3 combined commit):
// tokenBucket is constructed via newTokenBucket; filterStats via newFilterStats
// when ctx.Stats is non-nil (nil-tolerance per ADR-0085 test-code path).
func buildRuntimeConfig(c *localratelimitv3.LocalRateLimit, ctx envoyhttp.FactoryCtx) (*runtimeConfig, error) {
	// Check 1 (PGV per §11.1): stat_prefix non-empty.
	statPrefix := c.GetStatPrefix()
	if statPrefix == "" {
		return nil, errors.New("local_ratelimit: stat_prefix required")
	}
	// Check 2 (PGV per §11.2a): token_bucket non-nil + max_tokens > 0.
	tb := c.GetTokenBucket()
	if tb == nil {
		return nil, errors.New("local_ratelimit: token_bucket required")
	}
	if tb.GetMaxTokens() == 0 {
		return nil, errors.New("local_ratelimit: token_bucket.max_tokens must be > 0")
	}
	maxTokens := int64(tb.GetMaxTokens())
	// Check 3 (PGV per §11.2b-i + §11.2b-ii): tokens_per_fill — absent → 1; explicit zero → reject.
	var tokensPerFill int64
	if tb.GetTokensPerFill() == nil {
		tokensPerFill = 1 // proto default per §11.2b-i empirical pin
	} else {
		v := tb.GetTokensPerFill().GetValue()
		if v == 0 {
			return nil, errors.New("local_ratelimit: token_bucket.tokens_per_fill must be > 0 if specified")
		}
		tokensPerFill = int64(v)
	}
	// Check 4 (FILTER-INTERNAL per §11.2c; NOT PGV; verbatim Envoy error string):
	// fill_interval must be >= 50ms.
	fillInterval := tb.GetFillInterval().AsDuration()
	if fillInterval < minFillInterval {
		// Verbatim Envoy v1.37.2 error string from source/server/config_validation/server.cc:76.
		// The mirrored string is the LOAD-BEARING wire-equivalence claim per ADR-0115:
		// operators reading boot logs see identical failure shapes across reference + envoy-go.
		return nil, errors.New("local rate limit token bucket fill timer must be >= 50ms")
	}
	// Check 5 (PGV per §11.4): status.code — absent → 429; explicit out-of-[400, 600) → reject.
	statusCode := 429
	if c.GetStatus() != nil {
		statusCode = int(c.GetStatus().GetCode())
		if statusCode < 400 || statusCode >= 600 {
			return nil, fmt.Errorf("local_ratelimit: status.code must be in [400, 600); got %d", statusCode)
		}
	}
	// Construct the *tokenBucket (lazy-refill on access per ADR-0116).
	bucket := newTokenBucket(maxTokens, tokensPerFill, fillInterval)
	// Construct the *filterStats (Task 4 lands the counter-increment discipline;
	// Task 2+3 combined commit registers the counters; nil-tolerance for test code paths).
	var fs *filterStats
	if ctx.Stats != nil {
		fs = newFilterStats(ctx.Stats, statPrefix)
	}
	return &runtimeConfig{
		statPrefix: statPrefix,
		bucket:     bucket,
		statusCode: statusCode,
		body:       rateLimitedBody,
		stats:      fs,
	}, nil
}

// buildRuntimeConfigPerRoute is buildRuntimeConfig's per-route variant; it
// uses NewCounterIfAbsent for stats registration so per-route counters can
// be allocated post-Freeze (per ADR-0117).
//
// IMPL-1: takes *localratelimitv3.LocalRateLimit directly (no LocalRateLimitPerRoute
// wrapper — that proto does not exist in upstream Envoy v1.37.2; per-route TPFC
// entries reuse the same LocalRateLimit message per PROGRESS.md preamble IMPL-1).
func buildRuntimeConfigPerRoute(c *localratelimitv3.LocalRateLimit, reg *stats.Registry) (*runtimeConfig, error) {
	// Same 6 explicit checks as buildRuntimeConfig (callable independently).
	statPrefix := c.GetStatPrefix()
	if statPrefix == "" {
		return nil, errors.New("local_ratelimit: stat_prefix required")
	}
	tb := c.GetTokenBucket()
	if tb == nil {
		return nil, errors.New("local_ratelimit: token_bucket required")
	}
	if tb.GetMaxTokens() == 0 {
		return nil, errors.New("local_ratelimit: token_bucket.max_tokens must be > 0")
	}
	maxTokens := int64(tb.GetMaxTokens())
	var tokensPerFill int64 = 1
	if v := tb.GetTokensPerFill(); v != nil {
		if v.GetValue() == 0 {
			return nil, errors.New("local_ratelimit: token_bucket.tokens_per_fill must be > 0 if specified")
		}
		tokensPerFill = int64(v.GetValue())
	}
	fillInterval := tb.GetFillInterval().AsDuration()
	if fillInterval < minFillInterval {
		return nil, errors.New("local rate limit token bucket fill timer must be >= 50ms")
	}
	statusCode := 429
	if c.GetStatus() != nil {
		statusCode = int(c.GetStatus().GetCode())
		if statusCode < 400 || statusCode >= 600 {
			return nil, fmt.Errorf("local_ratelimit: status.code must be in [400, 600); got %d", statusCode)
		}
	}
	var fs *filterStats
	if reg != nil {
		fs = newFilterStatsIfAbsent(reg, statPrefix)
	}
	return &runtimeConfig{
		statPrefix: statPrefix,
		bucket:     newTokenBucket(maxTokens, tokensPerFill, fillInterval),
		statusCode: statusCode,
		body:       rateLimitedBody,
		stats:      fs,
	}, nil
}

// newFilterStats registers the 4 local_ratelimit stat counters on the supplied
// Registry per ADR-0118 + SPEC §11.5. Tolerates nil registry (test code per
// ADR-0085 nil-tolerance — callers guard with ctx.Stats != nil check above).
func newFilterStats(reg *stats.Registry, prefix string) *filterStats {
	p := prefix + ".http_local_rate_limit."
	return &filterStats{
		enabled:     reg.NewCounter(p + "enabled"),
		ok:          reg.NewCounter(p + "ok"),
		rateLimited: reg.NewCounter(p + "rate_limited"),
		enforced:    reg.NewCounter(p + "enforced"),
	}
}

// newFilterStatsIfAbsent constructs filterStats via NewCounterIfAbsent for
// post-Freeze idempotent registration (per ADR-0117).
func newFilterStatsIfAbsent(reg *stats.Registry, prefix string) *filterStats {
	p := prefix + ".http_local_rate_limit."
	return &filterStats{
		enabled:     reg.NewCounterIfAbsent(p + "enabled"),
		ok:          reg.NewCounterIfAbsent(p + "ok"),
		rateLimited: reg.NewCounterIfAbsent(p + "rate_limited"),
		enforced:    reg.NewCounterIfAbsent(p + "enforced"),
	}
}

// resolvePerRouteConfig returns the *runtimeConfig for the given resolved
// per-route TPFC message (returned by f.dcb.RequestRouteConfig()). If the
// message is *localratelimitv3.LocalRateLimit, it is used as a per-route
// config (lazily constructing a fresh *runtimeConfig + own *tokenBucket +
// own *filterStats keyed by the *LocalRateLimit pointer per ADR-0117 + IMPL-1).
// If nil, returns the listener fallback. Returns the listener fallback on
// any unexpected message type.
//
// IMPL-1: keyed by *localratelimitv3.LocalRateLimit (not *LocalRateLimitPerRoute
// — that message does not exist in upstream Envoy v1.37.2; per-route TPFC entries
// ARE LocalRateLimit directly per PROGRESS.md preamble IMPL-1).
func (s *factoryState) resolvePerRouteConfig(msg proto.Message) *runtimeConfig {
	if msg == nil {
		return s.listenerRC
	}
	perRoute, ok := msg.(*localratelimitv3.LocalRateLimit)
	if !ok {
		return s.listenerRC
	}
	if cached, ok := s.perRoute.Load(perRoute); ok {
		return cached.(*runtimeConfig)
	}
	// Lazy construction. Race-safe via sync.Map's LoadOrStore: if two goroutines
	// race to construct the per-route rc, only one allocation wins; the loser's
	// rc is GC'd (its registered counters are NOT GC'd since they're already in
	// the Registry, but they're idempotent and equivalent to the winner's via
	// NewCounterIfAbsent's pointer-identity guarantee, so the winner and loser
	// observe the same Counter instances).
	//
	// Future optimization for high-cardinality per-route maps under heavy initial
	// concurrency: a singleflight wrapper would dedupe the duplicate
	// buildRuntimeConfigPerRoute call. Not justified for the current low-
	// cardinality workload; revisit if profiling at fixture-0013-scale shows
	// repeated wasted allocations under cold-start fan-out.
	fresh, err := buildRuntimeConfigPerRoute(perRoute, s.reg)
	if err != nil {
		// Per-route TPFC parsing failed. Treat as "inherit listener" to keep
		// request flow alive (NO panic per ADR-0072 boot-fail-fast applies only
		// to boot-time, not request-time).
		return s.listenerRC
	}
	actual, _ := s.perRoute.LoadOrStore(perRoute, fresh)
	return actual.(*runtimeConfig)
}

// DecodeHeaders implements the rate-limit decision per SPEC §6.5 + ADR-0119.
// Resolves per-route TPFC via dcb.RequestRouteConfig() (most-specific accessor
// per ADR-0073); unwraps to *runtimeConfig via state.resolvePerRouteConfig.
//
// Discipline:
//  1. Resolve per-route config via dcb.RequestRouteConfig(); fall back to listener.
//  2. Increment rc.stats.enabled (unconditional per-request).
//  3. Call rc.bucket.tryConsume().
//  4. If true: increment rc.stats.ok; return Continue.
//  5. If false: increment rc.stats.rateLimited AND rc.stats.enforced
//     IN LOCKSTEP (MVP invariant per ADR-0118; future shadow-mode phase
//     widens to enforced ≤ rate_limited when filter_enforced runtime-key
//     support lands per the Runtime + hot restart family).
//  6. Invoke f.dcb.SendLocalReply(rc.statusCode, rc.body, OrderedHeaders{
//     {Name: "Content-Type", Value: "text/plain"}}) per ADR-0102 + ADR-0119.
//     Reuses the request-side terminal-replace primitive verbatim from phase
//     09 fault precedent at internal/filter/http/fault/fault.go:321.
//  7. Return StopIteration.
func (f *filter) DecodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	var perRouteMsg proto.Message
	if f.dcb != nil {
		perRouteMsg = f.dcb.RequestRouteConfig()
	}
	rc := f.state.resolvePerRouteConfig(perRouteMsg)
	rc.stats.enabled.Inc()
	if rc.bucket.tryConsume() {
		rc.stats.ok.Inc()
		return envoyhttp.Continue
	}
	rc.stats.rateLimited.Inc()
	rc.stats.enforced.Inc() // lockstep MVP per ADR-0118
	f.dcb.SendLocalReply(rc.statusCode, rc.body, envoyhttp.OrderedHeaders{
		{Name: "Content-Type", Value: "text/plain"},
	})
	return envoyhttp.StopIteration
}

// Pass-through methods per SPEC §6.5. local_ratelimit operates only on
// DecodeHeaders; all other states are no-op.
func (f *filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
func (f *filter) EncodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	return envoyhttp.Continue
}
func (f *filter) EncodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}
func (f *filter) EncodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
func (f *filter) OnDestroy() {}
