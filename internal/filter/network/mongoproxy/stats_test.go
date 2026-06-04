package mongoproxy

import (
	"sort"
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

// goldenRoster is the EXACT 22 counter suffixes transcribed from upstream
// ALL_MONGO_PROXY_STATS (parent §7.2 / R2). delays_injected is PLURAL (AMEND-B3
// regression guard). DO NOT edit to match a code change.
var goldenRoster = []string{
	"cx_destroy_local_with_active_rq", "cx_destroy_remote_with_active_rq",
	"cx_drain_close", "decoding_error", "delays_injected",
	"op_command", "op_command_reply", "op_get_more", "op_insert", "op_kill_cursors",
	"op_query", "op_query_await_data", "op_query_exhaust", "op_query_multi_get",
	"op_query_no_cursor_timeout", "op_query_no_max_time", "op_query_scatter_get",
	"op_query_tailable_cursor", "op_reply", "op_reply_cursor_not_found",
	"op_reply_query_failure", "op_reply_valid_cursor",
}

func TestStatRoster_MatchesUpstreamMacro(t *testing.T) {
	got := append([]string(nil), rosterSuffixes()...)
	sort.Strings(got)
	want := append([]string(nil), goldenRoster...)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("roster has %d suffixes, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("suffix[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestStatRoster_EagerCreation(t *testing.T) {
	reg := stats.NewRegistry()
	ms := newMongoStats(reg, "mongo_a")
	// All 22 counters + the gauge exist under mongo.mongo_a. before any traffic,
	// at value 0 (the zk TestRosterStats_EagerCreation pattern — check the struct
	// map directly; the registry exposes no Lookup, only Walk).
	if len(ms.counters) != 22 {
		t.Fatalf("created %d counters, want 22", len(ms.counters))
	}
	for _, suf := range goldenRoster {
		c := ms.counters[suf]
		if c == nil {
			t.Errorf("counter %q not created eagerly", suf)
			continue
		}
		if c.Load() != 0 {
			t.Errorf("counter %q = %d at creation, want 0", suf, c.Load())
		}
	}
	if ms.opQueryActive == nil || ms.opQueryActive.Load() != 0 {
		t.Errorf("gauge op_query_active not created eagerly at 0")
	}
}

func TestStatRoster_IdempotentSharedPrefix(t *testing.T) {
	// Two listeners sharing a stat_prefix share counter instances (no panic) —
	// the zk TestRosterStats_IdempotentSharedPrefix precedent.
	reg := stats.NewRegistry()
	a := newMongoStats(reg, "mongo_a")
	b := newMongoStats(reg, "mongo_a")
	if a.counters["op_query"] != b.counters["op_query"] {
		t.Fatal("shared stat_prefix must share the same counter instances")
	}
}

func TestStatRoster_DynamicNames(t *testing.T) {
	reg := stats.NewRegistry()
	ms := newMongoStats(reg, "p")
	// The helpers register lazily; verify the registered NAME via Counter.Name()
	// (Metric.Name() — the zk dynamic-auth test pattern).
	cases := map[*stats.Counter]string{
		ms.cmdTotal("isMaster"):                               "mongo.p.cmd.isMaster.total",
		ms.collectionQuery("collection1", "scatter_get"):      "mongo.p.collection.collection1.query.scatter_get",
		ms.callsiteQuery("collection1", "fixtureFn", "total"): "mongo.p.collection.collection1.callsite.fixtureFn.query.total",
	}
	for c, want := range cases {
		if c.Name() != want {
			t.Errorf("dynamic counter name = %q, want %q", c.Name(), want)
		}
	}
}
