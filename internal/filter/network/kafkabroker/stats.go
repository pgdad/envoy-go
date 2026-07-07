package kafkabroker

import (
	"github.com/pgdad/envoy-go/internal/filter/network/statroster"
	"github.com/pgdad/envoy-go/internal/stats"
)

// kafkaStats holds the EAGER 176-counter roster (AMEND-K3): 86 per-key request
// counters + 86 per-key response counters + 4 fixed (request.unknown /
// request.failure / response.unknown / response.failure), all created under
// kafka.<stat_prefix>. via NewCounterIfAbsent — idempotent across listeners
// sharing a stat_prefix. The 86 response-duration histograms are DEFERRED
// (ADR-0060) and NOT created here.
type kafkaStats struct {
	prefix   string                    // "kafka.<stat_prefix>."
	counters map[string]*stats.Counter // keyed by the suffix after kafka.<sp>.
}

// fixedSuffixes are the 4 non-per-key roster entries.
var fixedSuffixes = []string{"request.unknown", "request.failure", "response.unknown", "response.failure"}

// rosterSuffixes builds the full 176-suffix table: 86 per-key request +
// 86 per-key response + the 4 fixed entries (the kafkaStats doc order).
func rosterSuffixes() []string {
	roots := apiKeyRoster()
	out := make([]string, 0, 2*len(roots)+len(fixedSuffixes))
	for _, root := range roots {
		out = append(out, "request."+root+"_request", "response."+root+"_response")
	}
	return append(out, fixedSuffixes...)
}

// newKafkaStats eagerly creates all 176 counters under kafka.<statPrefix>. via
// the shared statroster scaffold (NewCounterIfAbsent — post-Freeze-permitted and
// idempotent across listeners sharing a stat_prefix; the mongoproxy precedent).
func newKafkaStats(reg *stats.Registry, statPrefix string) *kafkaStats {
	prefix := "kafka." + statPrefix + "."
	return &kafkaStats{
		prefix:   prefix,
		counters: statroster.New(reg, prefix, rosterSuffixes()),
	}
}

// inc increments the roster counter for suffix. Unknown suffix is a programming
// error → panic (the roster is closed and eager).
func (ks *kafkaStats) inc(suffix string) { statroster.Inc("kafkabroker", ks.counters, suffix) }

func (ks *kafkaStats) incRequest(root string)  { ks.inc("request." + root + "_request") }
func (ks *kafkaStats) incResponse(root string) { ks.inc("response." + root + "_response") }
func (ks *kafkaStats) incRequestUnknown()      { ks.inc("request.unknown") }
func (ks *kafkaStats) incRequestFailure()      { ks.inc("request.failure") }
func (ks *kafkaStats) incResponseUnknown()     { ks.inc("response.unknown") }
func (ks *kafkaStats) incResponseFailure()     { ks.inc("response.failure") }
