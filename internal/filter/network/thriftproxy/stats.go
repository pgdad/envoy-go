package thriftproxy

import (
	"github.com/pgdad/envoy-go/internal/filter/network/statroster"
	"github.com/pgdad/envoy-go/internal/stats"
)

// counterSuffixes is the fixed 24-counter roster under thrift.<stat_prefix>.
// Pinned name-for-name against ALL_THRIFT_FILTER_STATS + the 5 router counters
// (SPEC §7.2 / §11.3). The 2 close-direction counters + downstream_response_drain
// _close are created but NEVER incremented (AMEND-T6); the request_internal_error/
// shadow_request_submit_failure/upstream_rq_maintenance_mode/downstream_cx_max
// _requests members exist-at-0 (deferred features).
var counterSuffixes = []string{
	"request",
	"request_call",
	"request_oneway",
	"request_passthrough",
	"request_decoding_error",
	"request_invalid_type",
	"request_internal_error",
	"response",
	"response_reply",
	"response_success",
	"response_error",
	"response_exception",
	"response_passthrough",
	"response_decoding_error",
	"response_invalid_type",
	"route_missing",
	"unknown_cluster",
	"no_healthy_upstream",
	"shadow_request_submit_failure",
	"upstream_rq_maintenance_mode",
	"cx_destroy_local_with_active_rq",  // exist-at-0, never incremented (AMEND-T6)
	"cx_destroy_remote_with_active_rq", // exist-at-0, never incremented (AMEND-T6)
	"downstream_cx_max_requests",       // exist-at-0 (max_requests_per_connection deferred)
	"downstream_response_drain_close",  // exist-at-0, never incremented (AMEND-T6)
}

// thriftStats holds the EAGER fixed roster (24 counters + 1 gauge) created under
// thrift.<prefix>. The request_time_ms histogram is DEFERRED (ADR-0060 — the
// registry has no Histogram type at all). Creation is via NewCounterIfAbsent /
// NewGaugeIfAbsent: post-Freeze-permitted and idempotent across listeners
// sharing a stat_prefix (the redisproxy/kafka/mongo precedent).
type thriftStats struct {
	prefix   string
	counters map[string]*stats.Counter
	active   *stats.Gauge
}

// newThriftStats eagerly creates the 24 fixed counters + the request_active gauge
// under thrift.<statPrefix>. so the differential /stats scrape sees them present-
// at-0 even when never incremented.
func newThriftStats(reg *stats.Registry, statPrefix string) *thriftStats {
	prefix := "thrift." + statPrefix + "."
	return &thriftStats{
		prefix:   prefix,
		counters: statroster.New(reg, prefix, counterSuffixes),
		active:   reg.NewGaugeIfAbsent(prefix + "request_active"),
	}
}

// inc increments the roster counter for suf (the Task-8 pump's classify verdicts).
func (ts *thriftStats) inc(suf string) { statroster.Inc("thriftproxy", ts.counters, suf) }

// incActive / decActive drive the request_active gauge (request-received /
// reply-sent in the Task-8 pump).
func (ts *thriftStats) incActive() { ts.active.Inc() }
func (ts *thriftStats) decActive() { ts.active.Dec() }
