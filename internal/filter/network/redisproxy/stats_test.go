package redisproxy

import (
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

func TestStatRoster32_1_MatchesUpstream(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	if got := len(rs.counters); got != 6 {
		t.Fatalf("counter roster size = %d, want 6", got)
	}
	if got := len(rs.gauges); got != 4 {
		t.Fatalf("gauge roster size = %d, want 4", got)
	}
	// The 10 names match upstream ALL_REDIS_PROXY_STATS (the 32.1 subset, R2).
	for _, suf := range []string{
		"downstream_cx_total", "downstream_cx_drain_close", "downstream_cx_protocol_error",
		"downstream_cx_rx_bytes_total", "downstream_cx_tx_bytes_total", "downstream_rq_total",
	} {
		c, ok := rs.counters[suf]
		if !ok {
			t.Errorf("counter %q absent from eager roster", suf)
			continue
		}
		if c.Load() != 0 {
			t.Errorf("counter %q = %d at creation, want 0", suf, c.Load())
		}
	}
	for _, suf := range []string{
		"downstream_cx_active", "downstream_cx_rx_bytes_buffered",
		"downstream_cx_tx_bytes_buffered", "downstream_rq_active",
	} {
		if _, ok := rs.gauges[suf]; !ok {
			t.Errorf("gauge %q absent from eager roster", suf)
		}
	}
}

func TestStatRoster32_1_Idempotent(t *testing.T) {
	reg := stats.NewRegistry()
	a := newRedisStats(reg, "rp")
	b := newRedisStats(reg, "rp") // a second listener sharing the prefix — no panic, SAME instances
	if a.counters["downstream_cx_total"] != b.counters["downstream_cx_total"] {
		t.Fatal("shared stat_prefix must share the same counter instances")
	}
}

func TestStatRoster32_1_IncAccessors(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	rs.incCxTotal()
	rs.incRqTotal()
	rs.addRxBytes(7)
	rs.addTxBytes(3)
	if rs.counters["downstream_cx_total"].Load() != 1 {
		t.Error("incCxTotal")
	}
	if rs.counters["downstream_rq_total"].Load() != 1 {
		t.Error("incRqTotal")
	}
	if rs.counters["downstream_cx_rx_bytes_total"].Load() != 7 {
		t.Error("addRxBytes")
	}
	if rs.counters["downstream_cx_tx_bytes_total"].Load() != 3 {
		t.Error("addTxBytes")
	}
}
