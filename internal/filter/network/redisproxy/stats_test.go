package redisproxy

import (
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

func TestStatRoster32_1_MatchesUpstream(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	if got := len(rs.counters); got != 11 {
		t.Fatalf("counter roster size = %d, want 11", got)
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

func TestStatRoster32_2_PerCommandAndFixed(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	// 540 per-command counters (180 × 3 slots).
	for _, name := range supportedCommandList {
		for _, slot := range []string{"total", "success", "error"} {
			if c := rs.command(name, slot); c == nil {
				t.Errorf("per-command counter command.%s.%s absent", name, slot)
			} else if c.Load() != 0 {
				t.Errorf("command.%s.%s = %d at creation, want 0", name, slot, c.Load())
			}
		}
	}
	// 2 splitter + 3 REDIS_CLUSTER_STATS fixed counters.
	for _, suf := range []string{
		"splitter.invalid_request", "splitter.unsupported_command",
		"upstream_cx_drained", "max_upstream_unknown_connections_reached", "connection_rate_limited",
	} {
		if c, ok := rs.counters[suf]; !ok {
			t.Errorf("fixed counter %q absent", suf)
		} else if c.Load() != 0 {
			t.Errorf("fixed counter %q = %d at creation, want 0", suf, c.Load())
		}
	}
}

func TestStatRoster32_2_IncAccessors(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	rs.incCommandTotal("get")
	rs.incCommandSuccess("get")
	rs.incCommandError("set")
	rs.incSplitterInvalid()
	rs.incSplitterUnsupported()
	if rs.command("get", "total").Load() != 1 || rs.command("get", "success").Load() != 1 {
		t.Error("incCommandTotal/Success(get)")
	}
	if rs.command("set", "error").Load() != 1 {
		t.Error("incCommandError(set)")
	}
	if rs.counters["splitter.invalid_request"].Load() != 1 || rs.counters["splitter.unsupported_command"].Load() != 1 {
		t.Error("splitter inc accessors")
	}
}

func TestStatRoster32_2_GaugeAndProtocolError(t *testing.T) {
	reg := stats.NewRegistry()
	rs := newRedisStats(reg, "rp")
	rs.incCxActive()
	rs.incCxActive()
	rs.decCxActive()
	if got := rs.gauges["downstream_cx_active"].Load(); got != 1 {
		t.Errorf("downstream_cx_active = %d, want 1 (2 inc - 1 dec)", got)
	}
	rs.incRqActive()
	rs.decRqActive()
	if got := rs.gauges["downstream_rq_active"].Load(); got != 0 {
		t.Errorf("downstream_rq_active = %d, want 0 (balanced)", got)
	}
	rs.incProtocolError()
	if got := rs.counters["downstream_cx_protocol_error"].Load(); got != 1 {
		t.Errorf("downstream_cx_protocol_error = %d, want 1", got)
	}
	// The 2 buffered gauges stay exist-at-0 (coverage boundary — no inc/dec accessor).
	for _, suf := range []string{"downstream_cx_rx_bytes_buffered", "downstream_cx_tx_bytes_buffered"} {
		if got := rs.gauges[suf].Load(); got != 0 {
			t.Errorf("buffered gauge %q = %d, want 0 (framework-managed coverage boundary)", suf, got)
		}
	}
}
