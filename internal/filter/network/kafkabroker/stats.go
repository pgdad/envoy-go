package kafkabroker

import (
	"fmt"

	"github.com/esalaine/envoy-go/internal/stats"
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

// newKafkaStats eagerly creates all 176 counters under kafka.<statPrefix>. via
// NewCounterIfAbsent — post-Freeze-permitted and idempotent across listeners
// sharing a stat_prefix (the mongoproxy newMongoStats precedent).
func newKafkaStats(reg *stats.Registry, statPrefix string) *kafkaStats {
	ks := &kafkaStats{
		prefix:   "kafka." + statPrefix + ".",
		counters: make(map[string]*stats.Counter, 176),
	}
	for _, root := range apiKeyRoster() {
		reqSuf := "request." + root + "_request"
		respSuf := "response." + root + "_response"
		ks.counters[reqSuf] = reg.NewCounterIfAbsent(ks.prefix + reqSuf)
		ks.counters[respSuf] = reg.NewCounterIfAbsent(ks.prefix + respSuf)
	}
	for _, suf := range fixedSuffixes {
		ks.counters[suf] = reg.NewCounterIfAbsent(ks.prefix + suf)
	}
	return ks
}

// inc increments the roster counter for suffix. Unknown suffix is a programming
// error → panic (the roster is closed and eager).
func (ks *kafkaStats) inc(suffix string) {
	c, ok := ks.counters[suffix]
	if !ok {
		panic(fmt.Sprintf("kafkabroker: unknown roster suffix %q", suffix))
	}
	c.Inc()
}

func (ks *kafkaStats) incRequest(root string)  { ks.inc("request." + root + "_request") }
func (ks *kafkaStats) incResponse(root string) { ks.inc("response." + root + "_response") }
func (ks *kafkaStats) incRequestUnknown()      { ks.inc("request.unknown") }
func (ks *kafkaStats) incRequestFailure()      { ks.inc("request.failure") }
func (ks *kafkaStats) incResponseUnknown()     { ks.inc("response.unknown") }
func (ks *kafkaStats) incResponseFailure()     { ks.inc("response.failure") }
