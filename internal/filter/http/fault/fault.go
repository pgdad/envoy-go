package fault

import (
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"sync/atomic"
	"time"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	faultv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/fault/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// TypeURL is the canonical envoy.filters.http.fault typed_config type URL.
// Boot wiring in cmd/envoy-go/main.go (Task 8) registers New under this key
// in the HTTPRegistry per ADR-0072.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault"

// faultAbortBody is the byte-exact response body the abort path emits.
// 18 bytes; NO trailing newline (per SPEC §11.3/§11.4 empirical pin).
const faultAbortBody = "fault filter abort"

// faultStats holds the 5 fault.* stats registered at HCM-build time per
// ADR-0107. response_rl_injected is permanently zero in phase 09 (route A —
// future fault-extension phase or bandwidth_limit filter populates it).
type faultStats struct {
	abortsInjected     *stats.Counter
	delaysInjected     *stats.Counter
	faultsOverflow     *stats.Counter
	activeFaults       *stats.Gauge
	responseRLInjected *stats.Counter // permanently zero in phase 09
}

// runtimeConfig is the per-instance / per-route parsed config shape per ADR-0101.
//
// Six fields consumed at fault-eval time (delayEnabled / delayPercentage /
// delayFixedDelay / abortEnabled / abortPercentage / abortHTTPStatus +
// matchHeaders + maxActiveFaults). Eleven HTTPFault fields silently ignored
// per ADR-0104 / SPEC §2 deferrals (header_delay / header_abort / grpc_status /
// upstream_cluster / downstream_nodes / disable_downstream_cluster_stats /
// four runtime-key overrides / response_rate_limit / filter_metadata).
type runtimeConfig struct {
	delayEnabled    bool
	delayPercentage float64       // [0, 100]
	delayFixedDelay time.Duration // 0 if delay.header_delay set (silent-ignore path)

	abortEnabled    bool
	abortPercentage float64 // [0, 100]
	abortHTTPStatus int     // PGV-validated [200, 600) at New time

	matchHeaders []headerMatch // empty = match-all; only string_match.exact honored

	maxActiveFaults int64 // 0 = no cap
}

// headerMatch is one canonical-name + exact-value entry for the headers field.
type headerMatch struct {
	name       string // canonicalized via http.CanonicalHeaderKey at parse time
	exactValue string // string_match.exact (only matcher variant honored per §11.8)
}

// New is the HTTPFilterFactory exposed at boot. Per ADR-0100 + ADR-0101:
//
//  1. tc must be non-nil (a fault filter with no typed_config has no
//     behavioral effect; surface configuration mistakes at boot per
//     ADR-0072 boot-time-fail-fast).
//  2. Unmarshal tc to *faultv3.HTTPFault; return error on malformed Any.
//  3. Validate abort.http_status ∈ [200, 600) when abort.http_status set
//     per §11.1 PGV mirror.
//  4. Validate delay.fixed_delay > 0 when delay != nil AND delay.percentage > 0.
//  5. Construct *runtimeConfig per §6.2.
//  6. Allocate closure-captured *atomic.Int64 activeFaults counter (LBP-1
//     sixth; shared across per-instance values per ADR-0105).
//  7. Register the 5 fault.* stats on ctx.Stats keyed by
//     "http.<ctx.StatPrefix>.fault.<metric>" per ADR-0107.
//  8. Return FilterInstanceFactory closure that allocates a fresh *filter
//     per request bound to (cfg, active, stats).
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, errors.New("fault: typed_config required")
	}
	var c faultv3.HTTPFault
	if err := tc.UnmarshalTo(&c); err != nil {
		return nil, fmt.Errorf("fault: unmarshal: %w", err)
	}
	rc, err := parseRuntimeConfig(&c)
	if err != nil {
		return nil, err
	}
	activeFaults := new(atomic.Int64)
	fs := registerFaultStats(ctx.Stats, ctx.StatPrefix)
	return func() envoyhttp.HTTPFilter {
		f := &filter{
			cfg:    rc,
			active: activeFaults,
			stats:  fs,
			rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
		}
		return envoyhttp.HTTPFilter{
			Name:    "envoy.filters.http.fault",
			Decoder: f,
			Encoder: f,
		}
	}, nil
}

// parseRuntimeConfig projects the proto into the runtimeConfig shape per §6.2.
// Used by both New (with full validation) and parseRouteRuntimeConfig (Task 7
// adds per-route variant; the validation guards in this function fire on
// per-route inputs too — wholesale-override per §11.7 means per-route configs
// are independently validated).
func parseRuntimeConfig(c *faultv3.HTTPFault) (*runtimeConfig, error) {
	rc := &runtimeConfig{}
	if d := c.GetDelay(); d != nil {
		rc.delayPercentage = percentageToFloat(d.GetPercentage())
		rc.delayFixedDelay = d.GetFixedDelay().AsDuration()
		rc.delayEnabled = rc.delayFixedDelay > 0 // header_delay deferred per ADR-0104
		if rc.delayPercentage > 0 && rc.delayFixedDelay <= 0 {
			return nil, errors.New("fault: delay.fixed_delay required when delay.percentage > 0")
		}
	}
	if a := c.GetAbort(); a != nil {
		rc.abortPercentage = percentageToFloat(a.GetPercentage())
		// Only validate http_status when the HttpStatus oneof variant is set;
		// header_abort + grpc_status variants are silent-ignored per ADR-0104 /
		// ADR-0103 deferral.
		if _, ok := a.GetErrorType().(*faultv3.FaultAbort_HttpStatus); ok {
			hs := a.GetHttpStatus()
			// PGV mirror: must be in [200, 600).
			if hs < 200 || hs >= 600 {
				return nil, fmt.Errorf("fault: abort.http_status %d out of range [200, 600)", hs)
			}
			rc.abortHTTPStatus = int(hs)
			rc.abortEnabled = true
		}
	}
	if m := c.GetMaxActiveFaults(); m != nil {
		rc.maxActiveFaults = int64(m.GetValue())
	}
	if hs := c.GetHeaders(); len(hs) > 0 {
		rc.matchHeaders = make([]headerMatch, 0, len(hs))
		for _, h := range hs {
			sm, ok := h.GetHeaderMatchSpecifier().(*routev3.HeaderMatcher_StringMatch)
			if !ok || sm.StringMatch == nil {
				continue // non-string-match variants silent-ignored per §11.8 deferral
			}
			exact := sm.StringMatch.GetExact()
			if exact == "" {
				continue // non-exact StringMatcher variants silent-ignored
			}
			rc.matchHeaders = append(rc.matchHeaders, headerMatch{
				name:       http.CanonicalHeaderKey(h.GetName()),
				exactValue: exact,
			})
		}
	}
	return rc, nil
}

// percentageToFloat projects FractionalPercent into a float64 in [0, 100].
// Envoy's FractionalPercent denominator is one of HUNDRED / TEN_THOUSAND / MILLION.
func percentageToFloat(p *typev3.FractionalPercent) float64 {
	if p == nil {
		return 0
	}
	num := float64(p.GetNumerator())
	switch p.GetDenominator() {
	case typev3.FractionalPercent_HUNDRED:
		return num
	case typev3.FractionalPercent_TEN_THOUSAND:
		return num / 100.0
	case typev3.FractionalPercent_MILLION:
		return num / 10000.0
	}
	return 0
}

// registerFaultStats registers the 5 fault.* stats on the supplied Registry
// per ADR-0107. Tolerates nil registry (test code per ADR-0085 nil-tolerance).
func registerFaultStats(reg *stats.Registry, prefix string) *faultStats {
	if reg == nil {
		return &faultStats{} // all-nil; recordFaultEvent guards on nil
	}
	p := "http." + prefix + ".fault."
	return &faultStats{
		abortsInjected:     reg.NewCounter(p + "aborts_injected"),
		delaysInjected:     reg.NewCounter(p + "delays_injected"),
		faultsOverflow:     reg.NewCounter(p + "faults_overflow"),
		activeFaults:       reg.NewGauge(p + "active_faults"),
		responseRLInjected: reg.NewCounter(p + "response_rl_injected"),
	}
}

// filter is the per-request fault-filter instance. Per-instance state is
// race-free by the single-goroutine-per-stream invariant per ADR-0071.
// Tasks 4–7 fill in the DecodeHeaders body, the timer wiring, the per-route
// resolution, and the markedActive guard.
type filter struct {
	cfg    *runtimeConfig
	active *atomic.Int64
	stats  *faultStats
	rng    *rand.Rand

	dcb envoyhttp.DecoderFilterCallbacks
	ecb envoyhttp.EncoderFilterCallbacks

	delayTimer   *time.Timer //nolint:unused // Task 5 (ADR-0102) async-resume timer; Task 3 lands the field.
	markedActive bool        //nolint:unused // Task 6 (ADR-0105) markedActive guard; Task 3 lands the field.
}

// Statically assert the both-sides interface conformance (matches cors precedent).
var (
	_ envoyhttp.StreamDecoderFilter = (*filter)(nil)
	_ envoyhttp.StreamEncoderFilter = (*filter)(nil)
)

func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }

// DecodeHeaders implements the fault filter's decode-side discipline per SPEC §6.4.
// Task 4 lands the abort-only path + headers gate + percentage roll. Tasks 5/6/7
// fill in delay async-resume, max_active_faults Inc/Dec, and per-route 3-tier merge.
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	cfg := f.cfg // Task 7 replaces with f.routeConfigOrListener()
	if !f.matchesHeaders(headers, cfg) {
		return envoyhttp.Continue
	}
	delayApplies := cfg.delayEnabled && f.rollPercent(cfg.delayPercentage)
	abortApplies := cfg.abortEnabled && f.rollPercent(cfg.abortPercentage)
	if !delayApplies && !abortApplies {
		return envoyhttp.Continue
	}
	// Task 6 inserts max_active_faults cap check here:
	//   if cfg.maxActiveFaults > 0 && f.active.Load() >= cfg.maxActiveFaults {
	//       f.recordFaultEvent(eventFaultsOverflow)
	//       return envoyhttp.Continue
	//   }
	//   f.markActive()
	if delayApplies && abortApplies {
		// Combined path lands in Task 5.
		return envoyhttp.Continue // placeholder; Task 5 replaces
	}
	if delayApplies {
		// Delay-only path lands in Task 5.
		return envoyhttp.Continue // placeholder; Task 5 replaces
	}
	// Abort-only path (Task 4 scope).
	f.recordFaultEvent(eventAbortsInjected)
	f.dcb.SendLocalReply(cfg.abortHTTPStatus, faultAbortBody, envoyhttp.OrderedHeaders{
		{Name: "Content-Type", Value: "text/plain"},
	})
	return envoyhttp.StopIteration
}

// matchesHeaders returns true if cfg.matchHeaders is empty (match-all) OR
// every (canonical-name, exactValue) pair has a matching request-header
// VALUE under case-sensitive byte-equality (per §11.8 conclusion (a)+(b)).
// Header NAMES match case-insensitively via http.CanonicalHeaderKey
// (parse-time canonicalization in parseRuntimeConfig + http.Header.Get's
// runtime canonicalization — both sides land on the same canonical key).
func (f *filter) matchesHeaders(headers http.Header, cfg *runtimeConfig) bool {
	if len(cfg.matchHeaders) == 0 {
		return true
	}
	for _, hm := range cfg.matchHeaders {
		if headers.Get(hm.name) != hm.exactValue {
			return false
		}
	}
	return true
}

// rollPercent returns true iff a fresh random sample falls under p (in [0, 100]).
// Per planner-time decision 12: 0 short-circuits to false; 100 short-circuits
// to true; intermediate values consult the per-instance *rand.Rand seeded by
// time.Now().UnixNano() at filter-instance allocation time. The short-circuits
// preserve determinism at the boundary percentages (0% never fires; 100%
// always fires) without consulting the RNG — important for the
// decode-goroutine-only RNG-access invariant per ADR-0102.
func (f *filter) rollPercent(p float64) bool {
	if p <= 0 {
		return false
	}
	if p >= 100 {
		return true
	}
	return f.rng.Float64()*100 < p
}

// faultEventKind enumerates stat-event kinds dispatched through recordFaultEvent
// per planner-time decision 3 (consolidated stat-call-site).
type faultEventKind int

// faultEventKind enum values. Tasks 4/5/6 increment the relevant counters via
// recordFaultEvent rather than directly touching f.stats.X.Inc()/Dec() —
// consolidation makes the test surface (counter equality at the
// http.<sp>.fault.<metric> name) the single observable point.
const (
	eventAbortsInjected faultEventKind = iota
	eventDelaysInjected
	eventFaultsOverflow
	eventActiveFaultsInc
	eventActiveFaultsDec
)

// recordFaultEvent dispatches the stat-counter Inc/Dec per planner-time decision 3
// (consolidated stat-call-site). Tolerates nil stats (test code per ADR-0085);
// per-counter nil-guards tolerate the all-nil faultStats produced by the
// nil-registry path in registerFaultStats.
func (f *filter) recordFaultEvent(k faultEventKind) {
	if f.stats == nil {
		return
	}
	switch k {
	case eventAbortsInjected:
		if f.stats.abortsInjected != nil {
			f.stats.abortsInjected.Inc()
		}
	case eventDelaysInjected:
		if f.stats.delaysInjected != nil {
			f.stats.delaysInjected.Inc()
		}
	case eventFaultsOverflow:
		if f.stats.faultsOverflow != nil {
			f.stats.faultsOverflow.Inc()
		}
	case eventActiveFaultsInc:
		if f.stats.activeFaults != nil {
			f.stats.activeFaults.Inc()
		}
	case eventActiveFaultsDec:
		if f.stats.activeFaults != nil {
			f.stats.activeFaults.Dec()
		}
	}
}

// decrementActive is the markedActive-guarded per-instance Inc/Dec balance helper.
// Task 6 wires the markedActive bool + the Inc/Dec calls fully; Task 4 lands
// the helper as the (no-Inc-yet) Dec-side stub so OnDestroy and timer-
// callback paths added in Tasks 5/6 can call it without sequencing concerns.
// The markedActive bool gate guarantees Inc/Dec balance under double-call
// (e.g. if both timer-callback AND OnDestroy fire, only the first call
// performs the Dec).
//
//nolint:unused // Task 6 (ADR-0105) consumes from OnDestroy + timer callback; Task 4 lands the helper.
func (f *filter) decrementActive() {
	if f.markedActive {
		f.markedActive = false
		f.active.Add(-1)
		f.recordFaultEvent(eventActiveFaultsDec)
	}
}

// Encode-side and data/trailer methods are no-op pass-through.
func (f *filter) DecodeData([]byte, bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
func (f *filter) DecodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
func (f *filter) EncodeHeaders(http.Header, bool) envoyhttp.FilterHeadersStatus {
	return envoyhttp.Continue
}
func (f *filter) EncodeData([]byte, bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
func (f *filter) EncodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// OnDestroy is a stub at Task 3; Task 6 fills in the timer-cancel + Dec.
func (f *filter) OnDestroy() {}
