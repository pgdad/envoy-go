package mongoproxy

import (
	"github.com/pgdad/envoy-go/internal/filter/network/statroster"
	"github.com/pgdad/envoy-go/internal/stats"
)

// rosterSuffixes returns the EXACT 22 upstream-macro counter suffixes
// (parent §7.2; ALL_MONGO_PROXY_STATS; R2). delays_injected is PLURAL
// (AMEND-B3). The gauge op_query_active is created separately (newMongoStats).
// Construction order is not sorted; the R2 test sorts before comparing.
func rosterSuffixes() []string {
	return []string{
		"cx_destroy_local_with_active_rq", "cx_destroy_remote_with_active_rq",
		"cx_drain_close", "decoding_error", "delays_injected",
		"op_command", "op_command_reply", "op_get_more", "op_insert", "op_kill_cursors",
		"op_query", "op_query_await_data", "op_query_exhaust", "op_query_multi_get",
		"op_query_no_cursor_timeout", "op_query_no_max_time", "op_query_scatter_get",
		"op_query_tailable_cursor", "op_reply", "op_reply_cursor_not_found",
		"op_reply_query_failure", "op_reply_valid_cursor",
	}
}

// mongoStats holds the 23 eagerly-created fixed stats (22 counters + the
// op_query_active gauge; D-P1 creation parity — created once per distinct
// stat_prefix at config parse) plus the lazily-created dynamic counter families.
type mongoStats struct {
	prefix        string // "mongo.<stat_prefix>."
	reg           *stats.Registry
	counters      map[string]*stats.Counter // 22, keyed by suffix; all eager
	opQueryActive *stats.Gauge
}

// newMongoStats eagerly creates all 23 fixed stats under mongo.<statPrefix>. via
// the shared statroster scaffold + NewGaugeIfAbsent — post-Freeze-permitted and
// idempotent across listeners sharing a stat_prefix (the zookeeper newRosterStats
// precedent; D-P1). The boot-window departure vs upstream's per-connection
// creation is a BEHAVIOR_CONTRACT record (§7.5), unobservable to the differential.
func newMongoStats(reg *stats.Registry, statPrefix string) *mongoStats {
	prefix := "mongo." + statPrefix + "."
	return &mongoStats{
		prefix:        prefix,
		reg:           reg,
		counters:      statroster.New(reg, prefix, rosterSuffixes()),
		opQueryActive: reg.NewGaugeIfAbsent(prefix + "op_query_active"),
	}
}

// inc increments the fixed counter for suffix. Unknown suffix is a programming
// error → panic (the roster is closed; dynamic names go through the helpers).
func (ms *mongoStats) inc(suffix string) { statroster.Inc("mongoproxy", ms.counters, suffix) }

// cmdTotal returns mongo.<sp>.cmd.<cmd>.total (lazy; post-Freeze-permitted —
// the zookeeper auth.<scheme>_rq / rbac per-policy precedent). <cmd> is the
// remembered name or "unknown_command".
func (ms *mongoStats) cmdTotal(cmd string) *stats.Counter {
	return ms.reg.NewCounterIfAbsent(ms.prefix + "cmd." + cmd + ".total")
}

// collectionQuery returns mongo.<sp>.collection.<c>.query.<leaf> (leaf ∈
// total / scatter_get / multi_get).
func (ms *mongoStats) collectionQuery(c, leaf string) *stats.Counter {
	return ms.reg.NewCounterIfAbsent(ms.prefix + "collection." + c + ".query." + leaf)
}

// callsiteQuery returns mongo.<sp>.collection.<c>.callsite.<cs>.query.<leaf>
// (the AMEND-C3 double-count family — incremented IN ADDITION to collectionQuery).
func (ms *mongoStats) callsiteQuery(c, cs, leaf string) *stats.Counter {
	return ms.reg.NewCounterIfAbsent(ms.prefix + "collection." + c + ".callsite." + cs + ".query." + leaf)
}
