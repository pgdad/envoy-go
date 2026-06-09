package redisproxy

import "github.com/esalaine/envoy-go/internal/stats"

// counterSuffixes / gaugeSuffixes are the 32.1 subset of the parent §7.2 fixed-15
// roster: 6 downstream counters + 4 downstream gauges = 10 names under
// redis.<stat_prefix>. (the 2 splitter.* + 3 REDIS_CLUSTER_STATS + the EAGER
// per-command table are 32.2). Pinned name-for-name against ALL_REDIS_PROXY_STATS.
var counterSuffixes = []string{
	"downstream_cx_total",
	"downstream_cx_drain_close",
	"downstream_cx_protocol_error",
	"downstream_cx_rx_bytes_total",
	"downstream_cx_tx_bytes_total",
	"downstream_rq_total",
}

var gaugeSuffixes = []string{
	"downstream_cx_active",
	"downstream_cx_rx_bytes_buffered",
	"downstream_cx_tx_bytes_buffered",
	"downstream_rq_active",
}

// redisStats holds the EAGER 10-name 32.1 roster, created under redis.<prefix>.
// via NewCounterIfAbsent / NewGaugeIfAbsent — post-Freeze-permitted and
// idempotent across listeners sharing a stat_prefix (the kafka/mongo precedent).
type redisStats struct {
	prefix   string
	counters map[string]*stats.Counter
	gauges   map[string]*stats.Gauge
}

// newRedisStats eagerly creates the 10 fixed names under redis.<statPrefix>.
// (D-P32-1). The 4 gauges are created but NOT incremented at 32.1 (inc/dec is
// 32.2); the cx/rq counters increment in the Handle pump (filter.go).
func newRedisStats(reg *stats.Registry, statPrefix string) *redisStats {
	rs := &redisStats{
		prefix:   "redis." + statPrefix + ".",
		counters: make(map[string]*stats.Counter, len(counterSuffixes)),
		gauges:   make(map[string]*stats.Gauge, len(gaugeSuffixes)),
	}
	for _, suf := range counterSuffixes {
		rs.counters[suf] = reg.NewCounterIfAbsent(rs.prefix + suf)
	}
	for _, suf := range gaugeSuffixes {
		rs.gauges[suf] = reg.NewGaugeIfAbsent(rs.prefix + suf)
	}
	return rs
}

// The 32.1-incremented subset (§7.2). drain_close / protocol_error + the 4
// gauges' inc/dec are 32.2.
func (rs *redisStats) incCxTotal()      { rs.counters["downstream_cx_total"].Inc() }
func (rs *redisStats) incRqTotal()      { rs.counters["downstream_rq_total"].Inc() }
func (rs *redisStats) addRxBytes(n int) { rs.counters["downstream_cx_rx_bytes_total"].Add(uint64(n)) }
func (rs *redisStats) addTxBytes(n int) { rs.counters["downstream_cx_tx_bytes_total"].Add(uint64(n)) }
