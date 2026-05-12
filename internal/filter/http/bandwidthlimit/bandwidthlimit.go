package bandwidthlimit

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	bandwidthlimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/bandwidth_limit/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// TypeURL is the canonical envoy.filters.http.bandwidth_limit typed_config
// type URL. Boot wiring at cmd/envoy-go/main.go (Task 10) registers New under
// this key in the HTTPRegistry per ADR-0072 + ADR-0135.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.bandwidth_limit.v3.BandwidthLimit"

// filterName is the canonical http_filters[].name string for bandwidth_limit
// (matches the listener config typed_per_filter_config map keys).
const filterName = "envoy.filters.http.bandwidth_limit"

// defaultFillInterval is the proto-default fill_interval per Envoy filter
// source (50ms; per SPEC §1.1 amendment 5 + §11.P5).
const defaultFillInterval = 50 * time.Millisecond

// compiledConfig is the per-instance parsed config shape per ADR-0136.
//
// 4 fields consumed at runtime (statPrefix, enableMode, limitKbps,
// fillInterval, stats); 3 BandwidthLimit fields silent-ignored per ADR-0040
// + ADR-0136 §Decision (ii): runtime_enabled, enable_response_trailers,
// response_trailer_prefix.
//
// Closure-capture parse-at-New + read-only-shared-after-New per ADR-0101.
type compiledConfig struct {
	statPrefix   string                                     // PGV non-empty per §11.P2 + amendment 3 (REQUIRED min_len=1)
	enableMode   bandwidthlimitv3.BandwidthLimit_EnableMode // 4 values; default DISABLED
	limitKbps    uint64                                     // KiB/s per amendment 6; 0 sentinel = unset at listener (foot-gun per amendment 10)
	fillInterval time.Duration                              // PGV [20ms, 1s] when set; default 50ms per amendment 5
	stats        *filterStats                               // 14 active stats; nil when ctx.Stats is nil (test path per ADR-0085)
}

// filterStats is the 14-active-stat surface per ADR-0138 + SPEC §1.1
// amendment 7 (8 counters + 6 gauges). The 2 transfer-duration histograms
// that Envoy emits unconditionally are NOT registered in MVP per phase-06.1
// baseline + amendment 9 divergence-window (allow-listed via twin-series-
// filter at BEHAVIOR_CONTRACT §242).
//
// Counter / Gauge increment semantics land at Tasks 4-5; the struct
// declaration + skeleton newFilterStats helper land here at Task 2.
type filterStats struct {
	// 8 counters (cumulative across all streams).
	requestEnabled            *stats.Counter // bump at decode-side endStream-arrival with requestActive=true
	requestEnforced           *stats.Counter // bump by ticks at decode timer-fire (per-tick match per §11.P3)
	requestIncomingTotalSize  *stats.Counter // cumulative bytes entered decode-side
	requestAllowedTotalSize   *stats.Counter // cumulative bytes forwarded through decode-side
	responseEnabled           *stats.Counter // symmetric (encode side)
	responseEnforced          *stats.Counter // symmetric
	responseIncomingTotalSize *stats.Counter
	responseAllowedTotalSize  *stats.Counter
	// 6 gauges (transient per-stream state).
	requestPending       *stats.Gauge // count of streams waiting on decode timer
	requestIncomingSize  *stats.Gauge // bytes-buffered-but-not-yet-forwarded (transient)
	requestAllowedSize   *stats.Gauge // bytes-forwarded-this-tick (transient)
	responsePending      *stats.Gauge
	responseIncomingSize *stats.Gauge
	responseAllowedSize  *stats.Gauge
}

// factoryState is the closure-captured shared state per factory invocation.
// Mirrors phase-11 ADR-0117 + IMPL-1 pattern verbatim.
//
// listenerRC is the listener-level *compiledConfig (immutable post-
// construction); perRoute is a lazy-cache keyed by *bandwidthlimitv3.
// BandwidthLimit pointer (populated on first resolve via
// resolvePerRouteConfig per ADR-0117 + IMPL-1 + ADR-0125 §(xi) 6th canonical
// bare-message-via-TPFC); reg is captured for newFilterStatsIfAbsent at
// per-route lazy-resolve time per ADR-0117 post-Freeze idempotency.
type factoryState struct {
	listenerRC *compiledConfig
	perRoute   sync.Map // map[*bandwidthlimitv3.BandwidthLimit]*compiledConfig — per ADR-0117 + IMPL-1 + ADR-0125 §(xi)
	reg        *stats.Registry
}

// filter is the per-stream filter instance allocated by the factory closure.
// State is request-scoped; *factoryState is the closure-captured shared
// state (immutable post-construction except for the sync.Map lazy-cache;
// race-safe per sync.Map's contract).
//
// Both decode + encode-side state live on the same struct; ENCODER+DECODER
// HTTPFilter value pairs the same *filter on both sides per ADR-0135 +
// planner-time decision 9. Per-direction state is independent — decode and
// encode timers fire on separate goroutines; the goroutine-per-direction
// invariant means no cross-direction synchronization is needed.
//
// The full Encode/Decode body implementations land at Tasks 4 + 5; the
// OnDestroy + timer-cleanup discipline lands at Task 6.
type filter struct {
	state *factoryState
	dcb   envoyhttp.DecoderFilterCallbacks
	ecb   envoyhttp.EncoderFilterCallbacks

	// Decode-side state (per-stream; populated at DecodeHeaders +
	// DecodeData per SPEC §6.7). Wired live at Task 4 (this commit); the
	// requestTimer Stop-races-Fire OnDestroy discipline lands at Task 6.
	requestRC     *compiledConfig
	requestActive bool
	requestBody   []byte
	requestTimer  *time.Timer

	// Encode-side state (per-stream; populated at DecodeHeaders cascade +
	// EncodeData per SPEC §6.8). responseTimer's Stop-races-Fire OnDestroy
	// discipline lands at Task 6.
	responseRC     *compiledConfig
	responseActive bool
	responseBody   []byte
	responseTimer  *time.Timer
}

// Statically assert the both-sides interface conformance (matches the
// localratelimit + compressor precedent).
var (
	_ envoyhttp.StreamDecoderFilter = (*filter)(nil)
	_ envoyhttp.StreamEncoderFilter = (*filter)(nil)
)

// SetDecoderCallbacks / SetEncoderCallbacks per planner-time decision 10.
// The framework's per-stream chain calls each once per stream; the filter
// stores the reference for later use during DecodeHeaders / EncodeData /
// the timer-fire async-resume callbacks.
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }

// New is the HTTPFilterFactory exposed at boot. Per ADR-0135 + ADR-0136:
//
//  1. tc must be non-nil (a bandwidth_limit filter with no typed_config
//     surfaces a config mistake; boot-time-fail-fast per ADR-0072).
//  2. Unmarshal tc to *bandwidthlimitv3.BandwidthLimit; return error on
//     malformed Any.
//  3. Invoke buildCompiledConfig with isPerRoute=false (listener-level
//     parse-time PGV-mirror checks per SPEC §6.5 + ADR-0136 §Decision (i)-
//     (v)).
//  4. Capture *factoryState{listenerRC, reg} for per-route lazy-cache
//     (ADR-0117 + ADR-0139).
//  5. Return FilterInstanceFactory closure that allocates a fresh *filter
//     per stream bound to *factoryState. The HTTPFilter value sets
//     Decoder: f, Encoder: f — the SAME *filter instance services both
//     sides per ADR-0135 + planner-time decision 9 (mirrors phase-14
//     ADR-0129 generalized to symmetric BOTH-direction).
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, errors.New("bandwidth_limit: typed_config required")
	}
	var c bandwidthlimitv3.BandwidthLimit
	if err := tc.UnmarshalTo(&c); err != nil {
		return nil, fmt.Errorf("bandwidth_limit: unmarshal: %w", err)
	}
	rc, err := buildCompiledConfig(&c, ctx, false /*isPerRoute*/)
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

// buildCompiledConfig parses + validates the 4 consumed fields from the
// BandwidthLimit proto per SPEC §6.5 + ADR-0136 §Decision (i)-(vi). The
// isPerRoute flag toggles the CODE-LEVEL extra check at the per-route
// position for limit_kbps REQUIRED per amendment 4 + §11.P1 (the bare-
// message-via-TPFC discipline is the 6th canonical per-route pattern per
// ADR-0125 §(xi) + ADR-0139).
//
// PGV-mirror validation order matches Envoy filter source verbatim (per
// ADR-0136 §Decision (iv)); envoy-go-own error wording per ADR-0126
// clear-text-error-discipline precedent (e.g.,
// "bandwidth_limit: stat_prefix required" not Envoy's exact phrasing).
//
// stats allocation: nil-tolerance per ADR-0085 (ctx.Stats may be nil in
// test code paths). At the listener position newFilterStats is used (boot-
// time registration); at the per-route position newFilterStatsIfAbsent is
// used (post-Freeze idempotent per ADR-0117 + ADR-0139). The 14 active
// stats (8 counters + 6 gauges) under namespace `<prefix>.http_bandwidth_
// limit.<counter>` are finalized at Task 8 per ADR-0138; see
// newFilterStats / newFilterStatsIfAbsent below for the canonical name set.
func buildCompiledConfig(c *bandwidthlimitv3.BandwidthLimit, ctx envoyhttp.FactoryCtx, isPerRoute bool) (*compiledConfig, error) {
	// Check 1 (PGV mirror per §11.P2 + amendment 3): stat_prefix non-empty.
	statPrefix := c.GetStatPrefix()
	if statPrefix == "" {
		return nil, errors.New("bandwidth_limit: stat_prefix required")
	}
	// Check 2: enable_mode is one of {DISABLED, REQUEST, RESPONSE,
	// REQUEST_AND_RESPONSE}. PGV defined_only handles this at proto-decode;
	// no defensive mirror needed (the proto-typed enum can hold only valid
	// values after a successful Unmarshal).

	// Check 3 (PGV mirror per amendment 4): limit_kbps.
	var limitKbps uint64
	if c.GetLimitKbps() != nil {
		limitKbps = c.GetLimitKbps().GetValue()
		// PGV >= 1 enforced at proto-decode; defensive mirror.
		if limitKbps < 1 {
			return nil, errors.New("bandwidth_limit: limit_kbps must be >= 1")
		}
	} else if isPerRoute {
		// Per amendment 4 + §11.P1 + ADR-0136 §Decision (v) + ADR-0139:
		// at per-route level, limit_kbps is FILTER-INTERNAL REQUIRED. The
		// verbatim wording mirrors Envoy filter source
		// `"limit must be set for per route filter config"` rephrased per
		// envoy-go's stat-prefix-style error discipline (ADR-0126).
		return nil, errors.New("bandwidth_limit: per-route entry requires limit_kbps to be set")
	}

	// Check 4 (PGV mirror per amendment 5): fill_interval in [20ms, 1s].
	fillInterval := defaultFillInterval
	if c.GetFillInterval() != nil {
		fillInterval = c.GetFillInterval().AsDuration()
		if fillInterval < 20*time.Millisecond || fillInterval > 1*time.Second {
			return nil, fmt.Errorf("bandwidth_limit: fill_interval %v outside supported range [20ms, 1s]", fillInterval)
		}
	}

	// Silent-ignored fields (per ADR-0040 + ADR-0136 §Decision (ii)):
	//   - c.GetRuntimeEnabled() — parsed but always-100%-active per
	//     planner-time decision 7 + §11.P6.
	//   - c.GetEnableResponseTrailers() — parsed but always-no-trailers per
	//     planner-time decision 8 (trailer-emission framework primitive
	//     deferred to a future phase).
	//   - c.GetResponseTrailerPrefix() — couples to enable_response_trailers;
	//     parsed but not stored.

	var fs *filterStats
	if ctx.Stats != nil {
		if isPerRoute {
			fs = newFilterStatsIfAbsent(ctx.Stats, statPrefix) // post-Freeze idempotent per ADR-0117
		} else {
			fs = newFilterStats(ctx.Stats, statPrefix)
		}
	}

	return &compiledConfig{
		statPrefix:   statPrefix,
		enableMode:   c.GetEnableMode(),
		limitKbps:    limitKbps,
		fillInterval: fillInterval,
		stats:        fs,
	}, nil
}

// buildCompiledConfigPerRoute is the per-route variant invoked from
// resolvePerRouteConfig at lazy-cache time. Identical to buildCompiledConfig
// with isPerRoute=true except that the *stats.Registry pointer is passed
// directly (the per-route path has no envoyhttp.FactoryCtx in scope; the
// pointer is captured on factoryState at New time per ADR-0117 IMPL-1).
//
// Mirrors phase-11 buildRuntimeConfigPerRoute structurally; per ADR-0139
// (per-route INDEPENDENT-stats ratification — phase-15 is the SECOND row
// using stateful-override-with-INDEPENDENT-stats per ADR-0117 precedent).
// Consumed live from resolvePerRouteConfig (the lazy-cache LoadOrStore path).
func buildCompiledConfigPerRoute(c *bandwidthlimitv3.BandwidthLimit, reg *stats.Registry) (*compiledConfig, error) {
	// Thin wrapper: delegate to buildCompiledConfig with a synthetic
	// FactoryCtx carrying the captured registry pointer. The validation
	// path is identical; the per-route flag triggers the CODE-LEVEL
	// limit_kbps REQUIRED check + the newFilterStatsIfAbsent post-Freeze
	// idempotent stats registration path.
	return buildCompiledConfig(c, envoyhttp.FactoryCtx{Stats: reg}, true /*isPerRoute*/)
}

// parsePerRoute unmarshals the typed_per_filter_config Any into the bare
// *bandwidthlimitv3.BandwidthLimit proto per IMPL-1 + ADR-0125 §(xi) 6th
// canonical: phase-15 has NO BandwidthLimitPerRoute wrapper message in
// Envoy v1.37.2 (per §11.P1 empirical refutation); per-route TPFC entries
// reuse the same BandwidthLimit proto directly.
//
// Returns the parsed proto for the registry's per-route map; the actual
// PGV-mirror validation (including the CODE-LEVEL limit_kbps REQUIRED
// extra check per ADR-0136) happens at resolvePerRouteConfig lazy-cache
// time via buildCompiledConfigPerRoute.
func parsePerRoute(tc *anypb.Any) (proto.Message, error) {
	if tc == nil {
		return nil, errors.New("bandwidth_limit: per-route typed_config required")
	}
	var c bandwidthlimitv3.BandwidthLimit
	if err := tc.UnmarshalTo(&c); err != nil {
		return nil, fmt.Errorf("bandwidth_limit: per-route unmarshal: %w", err)
	}
	return &c, nil
}

// resolvePerRouteConfig returns the *compiledConfig for the given resolved
// per-route TPFC message (returned by f.dcb.RequestRouteConfig() at
// DecodeHeaders). Mirrors phase-11 resolvePerRouteConfig at
// internal/filter/http/localratelimit/local_ratelimit.go:305-337 verbatim,
// with *LocalRateLimit replaced by *BandwidthLimit and buildRuntimeConfig*
// by buildCompiledConfig*.
//
// Per ADR-0117 + ADR-0125 §(xi) + ADR-0139 IMPL-1: keyed by
// *bandwidthlimitv3.BandwidthLimit pointer-identity (NO BandwidthLimit-
// PerRoute wrapper exists in Envoy v1.37.2 per §11.P1). Race-safe via
// sync.Map's LoadOrStore contract.
//
// Productionized at Task 4 (DecodeHeaders per-route resolve, this commit);
// full lazy-cache parity + listener-vs-per-route stats parity test lands at
// Task 7 per ADR-0139.
func (s *factoryState) resolvePerRouteConfig(msg proto.Message) *compiledConfig {
	if msg == nil {
		return s.listenerRC
	}
	perRoute, ok := msg.(*bandwidthlimitv3.BandwidthLimit)
	if !ok {
		return s.listenerRC
	}
	if cached, ok := s.perRoute.Load(perRoute); ok {
		return cached.(*compiledConfig)
	}
	fresh, err := buildCompiledConfigPerRoute(perRoute, s.reg)
	if err != nil {
		// Per-route TPFC parsing failed at lazy resolve. Treat as inherit-
		// listener to keep request flow alive (per phase-11 precedent +
		// ADR-0072: boot-time-fail-fast applies at New, not at request
		// time; NO panic).
		return s.listenerRC
	}
	actual, _ := s.perRoute.LoadOrStore(perRoute, fresh)
	return actual.(*compiledConfig)
}

// newFilterStats registers the 14 bandwidth_limit stats on the supplied
// Registry per ADR-0138 + SPEC §6.2 + amendment 7 (8 counters + 6 gauges).
//
// **Caller MUST guard against nil registry**: this helper unconditionally
// dereferences reg via reg.NewCounter / reg.NewGauge and will nil-panic on
// the first call if invoked with reg == nil. The nil-tolerance contract lives
// at the SINGLE caller — see buildCompiledConfig above where `if ctx.Stats !=
// nil` gates entry — per ADR-0085 (registry-nil discipline is caller-side,
// not helper-side, to keep the helper's body straight-line).
//
// Namespace shape: <prefix>.http_bandwidth_limit.<counter> per §11.P11 +
// amendment 8 (underscore-infix; NOT HCM-rooted; same shape as phase-11
// local_ratelimit's <prefix>.http_local_rate_limit.<counter> per ADR-0118).
// Prometheus renders via internal/stats/name.go's inline-prefix detection as
// envoy_<prefix>_http_bandwidth_limit_<counter>{} with NO labels + NO tag-
// extractor (NOT a new SN-numbered rule per §11.P10 + ADR-0138 §Decision
// (iii); the inline-prefix detection sits in the default-branch fallback
// alongside the SN9 local_ratelimit detection but without label promotion).
//
// Finalized at Task 8 per ADR-0138. The 14 names below MUST stay in lockstep
// with internal/stats/name.go's blSegment switch (the canonical name set is
// duplicated there for Prometheus rendering — KEEP IN SYNC).
func newFilterStats(reg *stats.Registry, prefix string) *filterStats {
	p := prefix + ".http_bandwidth_limit."
	return &filterStats{
		// 8 counters.
		requestEnabled:            reg.NewCounter(p + "request_enabled"),
		requestEnforced:           reg.NewCounter(p + "request_enforced"),
		requestIncomingTotalSize:  reg.NewCounter(p + "request_incoming_total_size"),
		requestAllowedTotalSize:   reg.NewCounter(p + "request_allowed_total_size"),
		responseEnabled:           reg.NewCounter(p + "response_enabled"),
		responseEnforced:          reg.NewCounter(p + "response_enforced"),
		responseIncomingTotalSize: reg.NewCounter(p + "response_incoming_total_size"),
		responseAllowedTotalSize:  reg.NewCounter(p + "response_allowed_total_size"),
		// 6 gauges.
		requestPending:       reg.NewGauge(p + "request_pending"),
		requestIncomingSize:  reg.NewGauge(p + "request_incoming_size"),
		requestAllowedSize:   reg.NewGauge(p + "request_allowed_size"),
		responsePending:      reg.NewGauge(p + "response_pending"),
		responseIncomingSize: reg.NewGauge(p + "response_incoming_size"),
		responseAllowedSize:  reg.NewGauge(p + "response_allowed_size"),
	}
}

// newFilterStatsIfAbsent constructs filterStats via NewCounterIfAbsent +
// NewGaugeIfAbsent for post-Freeze idempotent registration of all 14 stats
// (8 counters + 6 gauges) per ADR-0117 + ADR-0138 §Decision (iv) + ADR-0139.
// The NewGaugeIfAbsent counterpart was added at Task 7 to extend phase-11's
// counter-only post-Freeze idempotency pattern to gauges, completing the per-
// route INDEPENDENT-stats discipline per ADR-0139 §Decision (iii).
//
// **Caller MUST guard against nil registry**: this helper unconditionally
// dereferences reg via reg.NewCounterIfAbsent / reg.NewGaugeIfAbsent and will
// nil-panic on the first call if invoked with reg == nil. The nil-tolerance
// contract lives at the SINGLE caller — see buildCompiledConfig above where
// `if ctx.Stats != nil` gates entry — per ADR-0085. The per-route path
// reaches this helper via buildCompiledConfigPerRoute which forwards the
// factoryState.reg pointer captured at New time (a non-nil invariant in
// production; tests guard explicitly).
//
// Re-registration against the same stat_prefix (e.g., multi-resolve against
// the same per-route TPFC pointer) returns pointer-identical Counter/Gauge
// instances per the Registry's byName map; the sync.Map lazy-cache at
// factoryState.perRoute makes this race-safe (LoadOrStore winner allocates
// fresh; LoadOrStore loser observes the winner's instances). Verified at
// Task 8 via TestStatsNamespace_NewFilterStatsIfAbsent_Idempotent.
func newFilterStatsIfAbsent(reg *stats.Registry, prefix string) *filterStats {
	p := prefix + ".http_bandwidth_limit."
	return &filterStats{
		// 8 counters via post-Freeze-idempotent NewCounterIfAbsent.
		requestEnabled:            reg.NewCounterIfAbsent(p + "request_enabled"),
		requestEnforced:           reg.NewCounterIfAbsent(p + "request_enforced"),
		requestIncomingTotalSize:  reg.NewCounterIfAbsent(p + "request_incoming_total_size"),
		requestAllowedTotalSize:   reg.NewCounterIfAbsent(p + "request_allowed_total_size"),
		responseEnabled:           reg.NewCounterIfAbsent(p + "response_enabled"),
		responseEnforced:          reg.NewCounterIfAbsent(p + "response_enforced"),
		responseIncomingTotalSize: reg.NewCounterIfAbsent(p + "response_incoming_total_size"),
		responseAllowedTotalSize:  reg.NewCounterIfAbsent(p + "response_allowed_total_size"),
		// 6 gauges via post-Freeze-idempotent NewGaugeIfAbsent (added at this
		// commit; mirrors NewCounterIfAbsent per ADR-0139).
		requestPending:       reg.NewGaugeIfAbsent(p + "request_pending"),
		requestIncomingSize:  reg.NewGaugeIfAbsent(p + "request_incoming_size"),
		requestAllowedSize:   reg.NewGaugeIfAbsent(p + "request_allowed_size"),
		responsePending:      reg.NewGaugeIfAbsent(p + "response_pending"),
		responseIncomingSize: reg.NewGaugeIfAbsent(p + "response_incoming_size"),
		responseAllowedSize:  reg.NewGaugeIfAbsent(p + "response_allowed_size"),
	}
}

// ---------------------------------------------------------------------------
// Stub Encode/Decode methods per SPEC §6.7 + §6.8 + §6.10. Bodies are
// pass-through Continue/DataContinue/TrailersContinue at Task 2; the real
// body algorithm + per-route resolution + timer-arm + counter increments
// land at Tasks 4 (decode-side) + 5 (encode-side) + 6 (OnDestroy + Stop-
// races-Fire). The skeleton lets the package compile cleanly + the
// StreamDecoderFilter + StreamEncoderFilter interface assertions hold.
// ---------------------------------------------------------------------------

// DecodeHeaders resolves the per-route config (or inherits listener) and
// caches the resulting *compiledConfig on f.requestRC. Cascades the same RC
// to f.responseRC (per-stream symmetric semantic per SPEC §6.7 — encode-side
// EncodeHeaders is a no-op since the RC is already cached here). Sets
// f.requestActive + f.responseActive per the resolved enable_mode.
//
// Always returns Continue: the throttle decision is body-driven (DecodeData),
// not header-driven; headers always pass.
func (f *filter) DecodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	var perRouteMsg proto.Message
	if f.dcb != nil {
		perRouteMsg = f.dcb.RequestRouteConfig()
	}
	f.requestRC = f.state.resolvePerRouteConfig(perRouteMsg)
	if f.requestRC == nil {
		// Defensive; resolvePerRouteConfig falls back to listenerRC which is
		// always non-nil after a successful New (per ADR-0072 boot-time-
		// fail-fast). This branch is structurally unreachable.
		return envoyhttp.Continue
	}
	em := f.requestRC.enableMode
	f.requestActive = em == bandwidthlimitv3.BandwidthLimit_REQUEST || em == bandwidthlimitv3.BandwidthLimit_REQUEST_AND_RESPONSE
	// Cache for encode-side use (per-stream, both directions share the same
	// resolved RC under symmetric semantics).
	f.responseRC = f.requestRC
	f.responseActive = em == bandwidthlimitv3.BandwidthLimit_RESPONSE || em == bandwidthlimitv3.BandwidthLimit_REQUEST_AND_RESPONSE
	return envoyhttp.Continue
}

// DecodeData implements the Path B-async buffer-then-delayed-emit body
// algorithm per SPEC §6.7 + ADR-0137. Inactive direction passes through;
// pre-endStream chunks accumulate on f.requestBody and return
// DataStopIterationAndBuffer; on endStream=true the kbps-per-tick
// throttleDuration is computed, *_enabled + *_incoming_total_size +
// *_incoming_size are bumped, and either the fast path (throttle==0; empty
// body) hits DataContinue immediately or the timer arm path bumps
// *_pending + schedules f.requestTimer via time.AfterFunc. The timer-fire
// callback increments *_enforced by exactly `ticks` (per-tick cumulative
// match per §11.P3 + planner-time decision 15), *_allowed_total_size +
// *_allowed_size, decrements *_pending, and invokes ContinueDecoding.
func (f *filter) DecodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
	if !f.requestActive {
		return envoyhttp.DataContinue
	}
	f.requestBody = append(f.requestBody, data...)
	if !endStream {
		// envoy-go HCM dispatch reads body chunks sequentially in the same
		// goroutine that runs the chain (per ADR-0076 + connection.go's
		// hasBody loop): a DataStopIterationAndBuffer here would deadlock
		// the dispatch goroutine (chain parks waiting for ContinueDecoding
		// that the same goroutine must dispatch). Accumulate locally and
		// return DataContinue — the framework's bodyBuf (connection.go)
		// holds the bytes for the post-chain upstream dial. The throttle
		// engages on the terminal (endStream=true) chunk below.
		return envoyhttp.DataContinue
	}
	// endStream=true: stream engaged → bump *_enabled + *_incoming_total_size + *_incoming_size.
	bodyLen := uint64(len(f.requestBody))
	if f.requestRC.stats != nil {
		f.requestRC.stats.requestEnabled.Inc()
		f.requestRC.stats.requestIncomingTotalSize.Add(bodyLen)
		f.requestRC.stats.requestIncomingSize.Set(int64(bodyLen)) // transient
	}
	throttle, ticks := throttleDuration(len(f.requestBody), f.requestRC.limitKbps, f.requestRC.fillInterval)
	if throttle == 0 {
		// No throttle needed (empty body — throttleDuration's only ==0 return is bodySize=0). Forward immediately.
		// *_pending NOT bumped on the fast path: the gauge tracks streams actively waiting on a timer; an empty-body stream never waits.
		if f.requestRC.stats != nil {
			f.requestRC.stats.requestAllowedTotalSize.Add(bodyLen)
			f.requestRC.stats.requestAllowedSize.Set(int64(bodyLen))
		}
		return envoyhttp.DataContinue
	}
	// Arm timer; bump *_pending; capture ticks for *_enforced at completion.
	if f.requestRC.stats != nil {
		f.requestRC.stats.requestPending.Inc()
	}
	f.requestTimer = time.AfterFunc(throttle, func() {
		if f.requestRC.stats != nil {
			f.requestRC.stats.requestEnforced.Add(ticks) // per-tick cumulative match per §11.P3 + planner-time decision 15
			f.requestRC.stats.requestAllowedTotalSize.Add(bodyLen)
			f.requestRC.stats.requestAllowedSize.Set(int64(bodyLen))
			f.requestRC.stats.requestPending.Dec()
		}
		f.dcb.ContinueDecoding()
	})
	return envoyhttp.DataStopIterationAndBuffer
}

// DecodeTrailers is a passthrough; bandwidth_limit does NOT emit trailers
// per planner-time decision 8 (always-no-trailers MVP; trailer-emission
// framework primitive deferred). Per SPEC §6.10.
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// EncodeHeaders is a 1-line no-op per SPEC §6.8: responseRC + responseActive
// were cached at DecodeHeaders via the per-stream symmetric cascade (see
// f.responseRC = f.requestRC at DecodeHeaders above). The encode-side path
// therefore does NOT re-resolve per-route — it consumes the already-cached
// state. The throttle decision is body-driven (EncodeData), not header-driven;
// headers always pass.
func (f *filter) EncodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	return envoyhttp.Continue
}

// EncodeData is the line-for-line mirror of DecodeData with response-side
// substitutions per SPEC §6.8. Implements the Path B-async buffer-then-delayed-
// emit body algorithm symmetric to the decode side: inactive direction passes
// through; pre-endStream chunks accumulate on f.responseBody and return
// DataStopIterationAndBuffer; on endStream=true the kbps-per-tick
// throttleDuration is computed, *_enabled + *_incoming_total_size +
// *_incoming_size are bumped, and either the fast path (throttle==0; empty
// body) hits DataContinue immediately or the timer arm path bumps *_pending +
// schedules f.responseTimer via time.AfterFunc. The timer-fire callback
// increments *_enforced by exactly `ticks` (per-tick cumulative match per
// §11.P3 + planner-time decision 15), *_allowed_total_size + *_allowed_size,
// decrements *_pending, and invokes ContinueEncoding.
//
// Framework-survey at Task 5 Step 4 confirmed: the DataStopIterationAndBuffer
// + ContinueEncoding path emits the accumulated bytes unchanged through the
// HCM post-chain consumer; cb.OverwriteBody is NOT invoked here (the same-
// bytes case per ADR-0137 §Decision (vi); the buffered bytes ARE the original
// bytes). ZERO-framework-deltas claim stays intact — phase-15 REUSES (not
// introduces) the existing chain primitives.
func (f *filter) EncodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
	if !f.responseActive {
		return envoyhttp.DataContinue
	}
	f.responseBody = append(f.responseBody, data...)
	if !endStream {
		// Symmetric mirror of the decode-side fix (see DecodeData above).
		// envoy-go HCM encode-side dispatch is synchronous in the same
		// goroutine as the chain: returning DataStopIterationAndBuffer
		// here would deadlock the goroutine. Accumulate locally; the
		// throttle engages on the terminal (endStream=true) chunk below.
		return envoyhttp.DataContinue
	}
	// endStream=true: stream engaged → bump *_enabled + *_incoming_total_size + *_incoming_size.
	bodyLen := uint64(len(f.responseBody))
	if f.responseRC.stats != nil {
		f.responseRC.stats.responseEnabled.Inc()
		f.responseRC.stats.responseIncomingTotalSize.Add(bodyLen)
		f.responseRC.stats.responseIncomingSize.Set(int64(bodyLen)) // transient
	}
	throttle, ticks := throttleDuration(len(f.responseBody), f.responseRC.limitKbps, f.responseRC.fillInterval)
	if throttle == 0 {
		// No throttle needed (empty body — throttleDuration's only ==0 return is bodySize=0). Forward immediately.
		// *_pending NOT bumped on the fast path: the gauge tracks streams actively waiting on a timer; an empty-body stream never waits.
		if f.responseRC.stats != nil {
			f.responseRC.stats.responseAllowedTotalSize.Add(bodyLen)
			f.responseRC.stats.responseAllowedSize.Set(int64(bodyLen))
		}
		return envoyhttp.DataContinue
	}
	// Arm timer; bump *_pending; capture ticks for *_enforced at completion.
	if f.responseRC.stats != nil {
		f.responseRC.stats.responsePending.Inc()
	}
	f.responseTimer = time.AfterFunc(throttle, func() {
		if f.responseRC.stats != nil {
			f.responseRC.stats.responseEnforced.Add(ticks) // per-tick cumulative match per §11.P3 + planner-time decision 15
			f.responseRC.stats.responseAllowedTotalSize.Add(bodyLen)
			f.responseRC.stats.responseAllowedSize.Set(int64(bodyLen))
			f.responseRC.stats.responsePending.Dec()
		}
		f.ecb.ContinueEncoding()
	})
	return envoyhttp.DataStopIterationAndBuffer
}

// EncodeTrailers is a passthrough; same reasoning as DecodeTrailers.
func (f *filter) EncodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// OnDestroy lands the Stop-races-Fire pending-gauge discipline per SPEC §6.9
// + §4 + planner-time decision 3 verbatim. For each direction:
//
//   - time.Timer.Stop() is documented atomic: it and the timer-callback are
//     mutually exclusive (per Go stdlib).
//   - Stop() returns true → the callback was PREVENTED from running → the arm-
//     side *_pending.Inc() is unbalanced; OnDestroy MUST Dec here to restore
//     balance.
//   - Stop() returns false → the callback already ran (or is about to run) →
//     the callback's own *_pending.Dec() handles the balance; OnDestroy MUST
//     NOT Dec to avoid double-decrement (which would underflow the gauge to
//     -1 across stream churn).
//
// The Stop() bool discriminator is race-clean by stdlib contract; phase-15
// adopts it directly per SPEC §6.9. Group 6's TestOnDestroy_RaceConcurrent_
// NoDoubleDecrement validates the discipline under N=100 concurrent OnDestroy
// + timer-fire iterations under `go test -race`. The fallback (markedActive
// atomic.Bool per phase-09 fault precedent at fault.go:441-465) is documented
// per planner-time decision 3 but NOT engaged: the simpler form held.
func (f *filter) OnDestroy() {
	if f.requestTimer != nil {
		if f.requestTimer.Stop() && f.requestRC != nil && f.requestRC.stats != nil {
			f.requestRC.stats.requestPending.Dec()
		}
	}
	if f.responseTimer != nil {
		if f.responseTimer.Stop() && f.responseRC != nil && f.responseRC.stats != nil {
			f.responseRC.stats.responsePending.Dec()
		}
	}
}
