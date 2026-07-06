package thriftproxy

import (
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

// TestStatRoster pins the EAGER 25-name roster (24 counters + 1 gauge) created
// under thrift.<stat_prefix>. (SPEC §7.2). The registry exposes no Lookup*
// accessor (and ADR-0060 reserves the histogram — there is no Histogram type),
// so presence is asserted via the thriftStats maps the redisproxy precedent.
func TestStatRoster(t *testing.T) {
	reg := stats.NewRegistry()
	st := newThriftStats(reg, "tp")

	if got := len(counterSuffixes); got != 24 {
		t.Fatalf("counter roster = %d, want 24", got)
	}
	if got := len(st.counters); got != 24 {
		t.Fatalf("created counter roster = %d, want 24", got)
	}
	// All 24 counters present-at-0 under thrift.tp.
	for _, suf := range counterSuffixes {
		c, ok := st.counters[suf]
		if !ok {
			t.Errorf("counter %q absent from eager roster", suf)
			continue
		}
		if c.Name() != "thrift.tp."+suf {
			t.Errorf("counter %q registered as %q, want thrift.tp.%s", suf, c.Name(), suf)
		}
		if c.Load() != 0 {
			t.Errorf("counter %q = %d at creation, want 0", suf, c.Load())
		}
	}
	// The request_active gauge is present-at-0.
	if st.active == nil {
		t.Fatal("request_active gauge not created")
	}
	if st.active.Name() != "thrift.tp.request_active" {
		t.Errorf("gauge registered as %q, want thrift.tp.request_active", st.active.Name())
	}
	if st.active.Load() != 0 {
		t.Errorf("request_active = %d at creation, want 0", st.active.Load())
	}
	// The request_time_ms histogram is NOT in the roster (ADR-0060).
	if _, ok := st.counters["request_time_ms"]; ok {
		t.Error("request_time_ms must NOT be created (ADR-0060)")
	}

	// Idempotent across a second prefix-sharing instance: SAME pointers, no panic.
	st2 := newThriftStats(reg, "tp")
	if st.counters["request"] != st2.counters["request"] {
		t.Error("shared stat_prefix must share the same counter instances")
	}
	if st.active != st2.active {
		t.Error("shared stat_prefix must share the same request_active gauge instance")
	}
}

// TestStatAccessors proves the inc/incActive/decActive accessors are live (the
// Task-8 pump consumes them).
func TestStatAccessors(t *testing.T) {
	reg := stats.NewRegistry()
	st := newThriftStats(reg, "tp")

	st.inc("request")
	st.inc("request")
	if got := st.counters["request"].Load(); got != 2 {
		t.Errorf("inc(request): got %d, want 2", got)
	}
	st.incActive()
	st.incActive()
	st.decActive()
	if got := st.active.Load(); got != 1 {
		t.Errorf("active after 2 inc / 1 dec: got %d, want 1", got)
	}
}
