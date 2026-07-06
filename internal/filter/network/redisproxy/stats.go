package redisproxy

import "github.com/pgdad/envoy-go/internal/stats"

// counterSuffixes is the fixed counter roster under redis.<stat_prefix>.: the 6
// downstream counters (32.1) + the 2 splitter.* + 3 REDIS_CLUSTER_STATS (32.2).
// Pinned name-for-name against ALL_REDIS_PROXY_STATS.
var counterSuffixes = []string{
	"downstream_cx_total",
	"downstream_cx_drain_close",
	"downstream_cx_protocol_error",
	"downstream_cx_rx_bytes_total",
	"downstream_cx_tx_bytes_total",
	"downstream_rq_total",
	// 32.2 fixed adds (§4.3):
	"splitter.invalid_request",
	"splitter.unsupported_command",
	"upstream_cx_drained",                      // REDIS_CLUSTER_STATS — exist-at-0 (§12.6)
	"max_upstream_unknown_connections_reached", // REDIS_CLUSTER_STATS — exist-at-0
	"connection_rate_limited",                  // REDIS_CLUSTER_STATS — exist-at-0
}

var gaugeSuffixes = []string{
	"downstream_cx_active",
	"downstream_cx_rx_bytes_buffered",
	"downstream_cx_tx_bytes_buffered",
	"downstream_rq_active",
}

// commandStat is the eager 3-counter block for one supported command (§4.1).
type commandStat struct {
	total   *stats.Counter
	success *stats.Counter
	errc    *stats.Counter
}

// redisStats holds the EAGER fixed roster (11 counters, 4 gauges, 32.1) plus the
// 540-counter per-command table (180 × 3, 32.2), all created under redis.<prefix>.
// via NewCounterIfAbsent / NewGaugeIfAbsent — post-Freeze-permitted and
// idempotent across listeners sharing a stat_prefix (the kafka/mongo precedent).
type redisStats struct {
	prefix   string
	counters map[string]*stats.Counter
	gauges   map[string]*stats.Gauge
	commands map[string]*commandStat // keyed by the lower-cased command name (§12.1)
}

// newRedisStats eagerly creates the 11 fixed counters + 4 gauges + 540
// per-command counters (180 × 3) under redis.<statPrefix>. (D-P32-1/32.2). The
// 4 gauges are created but NOT incremented at 32.1 (inc/dec is 32.2); the
// cx/rq counters increment in the Handle pump (filter.go).
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
	rs.commands = make(map[string]*commandStat, len(supportedCommandList))
	for _, name := range supportedCommandList {
		base := rs.prefix + "command." + name + "."
		rs.commands[name] = &commandStat{
			total:   reg.NewCounterIfAbsent(base + "total"),
			success: reg.NewCounterIfAbsent(base + "success"),
			errc:    reg.NewCounterIfAbsent(base + "error"),
		}
	}
	return rs
}

// The 32.1-incremented subset (§7.2). drain_close / protocol_error + the 4
// gauges' inc/dec are 32.2.
func (rs *redisStats) incCxTotal()      { rs.counters["downstream_cx_total"].Inc() }
func (rs *redisStats) incRqTotal()      { rs.counters["downstream_rq_total"].Inc() }
func (rs *redisStats) addRxBytes(n int) { rs.counters["downstream_cx_rx_bytes_total"].Add(uint64(n)) }
func (rs *redisStats) addTxBytes(n int) { rs.counters["downstream_cx_tx_bytes_total"].Add(uint64(n)) }

// command returns the per-command counter for (lower-cased name, slot∈{total,
// success,error}), or nil if the name is not a supported command (a classify
// invariant — the pump only calls these for actProxy verdicts with a table member).
func (rs *redisStats) command(name, slot string) *stats.Counter {
	cs, ok := rs.commands[name]
	if !ok {
		return nil
	}
	switch slot {
	case "total":
		return cs.total
	case "success":
		return cs.success
	case "error":
		return cs.errc
	}
	return nil
}

func (rs *redisStats) incCommandTotal(name string)   { rs.commands[name].total.Inc() }
func (rs *redisStats) incCommandSuccess(name string) { rs.commands[name].success.Inc() }
func (rs *redisStats) incCommandError(name string)   { rs.commands[name].errc.Inc() }

func (rs *redisStats) incSplitterInvalid()     { rs.counters["splitter.invalid_request"].Inc() }
func (rs *redisStats) incSplitterUnsupported() { rs.counters["splitter.unsupported_command"].Inc() }

// The 2 filter-driven lifecycle gauges (§4.4 / §12.3). cx_active: Handle entry/exit
// (LIVE-mirrored — the held-open arm §8.2). rq_active: request-received/reply-sent
// (quiesced-zero post-workload). The 2 *_bytes_buffered gauges get NO accessor —
// they are framework-buffer-managed upstream and stay exist-at-0 (coverage boundary).
func (rs *redisStats) incCxActive() { rs.gauges["downstream_cx_active"].Inc() }
func (rs *redisStats) decCxActive() { rs.gauges["downstream_cx_active"].Dec() }
func (rs *redisStats) incRqActive() { rs.gauges["downstream_rq_active"].Inc() }
func (rs *redisStats) decRqActive() { rs.gauges["downstream_rq_active"].Dec() }

// incProtocolError increments downstream_cx_protocol_error (§4.5) — wired at 32.2
// on the decode-error path (a malformed frame; the 32.1 pump returned silently).
func (rs *redisStats) incProtocolError() { rs.counters["downstream_cx_protocol_error"].Inc() }
