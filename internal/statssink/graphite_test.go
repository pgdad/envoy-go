package statssink

import (
	"strings"
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

// NOTE: udpListener(t) and sameSet(t, got, want) are defined in statsd_test.go
// (same package statssink) — reused directly here, not redefined.

func TestGraphiteStatsdSink_CounterAndGaugeTagsInName(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	c := reg.NewCounter("cluster.backend.upstream_rq_total")
	c.Add(7)
	g := reg.NewGauge("cluster.backend.membership_total")
	g.Set(1)

	s, err := NewGraphiteStatsdSink(addr, "grpfx", 0)
	if err != nil {
		t.Fatalf("NewGraphiteStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := read(2)
	want := []string{
		"grpfx.cluster.upstream_rq_total;envoy.cluster_name=backend:7|c",
		"grpfx.cluster.membership_total;envoy.cluster_name=backend:1|g",
	}
	sameSet(t, got, want)
}

func TestGraphiteStatsdSink_TwoTagNaturalOrder(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	rq2xx := reg.NewCounter("http.hcm_local.downstream_rq_2xx")
	rq2xx.Add(5)

	s, err := NewGraphiteStatsdSink(addr, "grpfx", 0)
	if err != nil {
		t.Fatalf("NewGraphiteStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := read(1)
	// Order matters here: response_code_class BEFORE http_conn_manager_prefix
	// (the SN4-prepend order) — a sort.Slice bug would emit
	// http_conn_manager_prefix first and this exact-literal assertion would
	// catch it.
	want := "grpfx.http.downstream_rq_xx;envoy.response_code_class=2;envoy.http_conn_manager_prefix=hcm_local:5|c"
	if len(got) != 1 || got[0] != want {
		t.Errorf("two-tag natural order: got %v, want [%s]", got, want)
	}
}

func TestGraphiteStatsdSink_UntaggedNoSemicolon(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	u := reg.NewCounter("server.dynamic_unknown_fields")
	u.Add(0)

	s, err := NewGraphiteStatsdSink(addr, "grpfx", 0)
	if err != nil {
		t.Fatalf("NewGraphiteStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := read(1)
	want := "grpfx.server.dynamic_unknown_fields:0|c"
	if len(got) != 1 || got[0] != want {
		t.Errorf("untagged: got %v, want [%s]", got, want)
	}
	if len(got) == 1 && strings.Contains(got[0], ";") {
		t.Errorf("untagged: datagram %q unexpectedly contains ';'", got[0])
	}
}

func TestGraphiteStatsdSink_DeltaSemanticsAcrossFlushes(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	c := reg.NewCounter("cluster.backend.upstream_rq_total")

	s, err := NewGraphiteStatsdSink(addr, "p", 0)
	if err != nil {
		t.Fatalf("NewGraphiteStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Flush 1: cumulative 7 → delta 7.
	c.Add(7)
	s.Submit(snapshot(reg, 0))
	got := read(1)
	want := "p.cluster.upstream_rq_total;envoy.cluster_name=backend:7|c"
	if len(got) != 1 || got[0] != want {
		t.Errorf("flush1: got %v, want [%s]", got, want)
	}

	// Flush 2: no new increments → delta 0 (zero-delta RE-EMITTED, proves the
	// sink-private deltaState is live).
	s.Submit(snapshot(reg, 0))
	got = read(1)
	want = "p.cluster.upstream_rq_total;envoy.cluster_name=backend:0|c"
	if len(got) != 1 || got[0] != want {
		t.Errorf("flush2 (idle): got %v, want [%s]", got, want)
	}

	// Flush 3: add 3 more (cumulative 10) → delta 3.
	c.Add(3)
	s.Submit(snapshot(reg, 0))
	got = read(1)
	want = "p.cluster.upstream_rq_total;envoy.cluster_name=backend:3|c"
	if len(got) != 1 || got[0] != want {
		t.Errorf("flush3: got %v, want [%s]", got, want)
	}
}

func TestGraphiteStatsdSink_IndependentDelta(t *testing.T) {
	grAddr, grRead := udpListener(t)
	dsdAddr, dsdRead := udpListener(t)

	reg := stats.NewRegistry()
	c := reg.NewCounter("cluster.backend.upstream_rq_total")
	c.Add(7)

	gr, err := NewGraphiteStatsdSink(grAddr, "p", 0)
	if err != nil {
		t.Fatalf("NewGraphiteStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = gr.Close() })

	dsd, err := NewDogStatsdSink(dsdAddr, "p", 0)
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = dsd.Close() })

	batch := snapshot(reg, 0)
	gr.Submit(batch)
	dsd.Submit(batch)

	got := grRead(1)
	want := "p.cluster.upstream_rq_total;envoy.cluster_name=backend:7|c"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("gr flush1: got %v, want [%s]", got, want)
	}
	gotDsd := dsdRead(1)
	wantDsd := "p.cluster.upstream_rq_total:7|c|#envoy.cluster_name:backend"
	if len(gotDsd) != 1 || gotDsd[0] != wantDsd {
		t.Fatalf("dsd flush1: got %v, want [%s]", gotDsd, wantDsd)
	}

	// Flush the DogStatsdSink an EXTRA time in between — the GraphiteStatsdSink's
	// next delta must be UNAFFECTED, proving no shared deltaState.
	dsd.Submit(snapshot(reg, 0))
	_ = dsdRead(1)

	c.Add(3)
	batch = snapshot(reg, 0)
	gr.Submit(batch)
	got = grRead(1)
	want = "p.cluster.upstream_rq_total;envoy.cluster_name=backend:3|c"
	if len(got) != 1 || got[0] != want {
		t.Errorf("gr flush2 after extra dsd flush: got %v, want [%s]", got, want)
	}
}

func TestGraphiteStatsdSink_GaugeAbsoluteAcrossFlushes(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	g := reg.NewGauge("cluster.backend.membership_total")

	s, err := NewGraphiteStatsdSink(addr, "p", 0)
	if err != nil {
		t.Fatalf("NewGraphiteStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Flush 1: absolute 1.
	g.Set(1)
	s.Submit(snapshot(reg, 0))
	got := read(1)
	want := "p.cluster.membership_total;envoy.cluster_name=backend:1|g"
	if len(got) != 1 || got[0] != want {
		t.Errorf("flush1: got %v, want [%s]", got, want)
	}

	// Flush 2: same value; must emit 1|g (absolute), NOT a 0 delta.
	g.Set(1)
	s.Submit(snapshot(reg, 0))
	got = read(1)
	if len(got) != 1 || got[0] != want {
		t.Errorf("flush2 (same value): got %v, want [%s]", got, want)
	}
}

func TestGraphiteStatsdSink_NegativeGauge(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	g := reg.NewGauge("cluster.backend.some_signed_gauge")
	g.Set(-5)

	s, err := NewGraphiteStatsdSink(addr, "p", 0)
	if err != nil {
		t.Fatalf("NewGraphiteStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := read(1)
	want := "p.cluster.some_signed_gauge;envoy.cluster_name=backend:-5|g"
	if len(got) != 1 || got[0] != want {
		t.Errorf("negative gauge: got %v, want [%s]", got, want)
	}
}

// TestGraphiteStatsdSink_BatchingExactBoundaryColocates is the LOAD-BEARING
// boundary probe: a cap set to EXACTLY the combined length of two lines
// (line1 + "\n" + line2) must co-locate both lines into ONE datagram — the
// boundary is INCLUSIVE (`prospective > cap`, STRICT).
func TestGraphiteStatsdSink_BatchingExactBoundaryColocates(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	reg.NewCounter("x").Add(1)
	reg.NewCounter("yy").Add(2)

	line1 := "p.x:1|c"
	line2 := "p.yy:2|c"
	if got, want := len(line1), 7; got != want {
		t.Fatalf("line1 %q: len = %d, want %d", line1, got, want)
	}
	if got, want := len(line2), 8; got != want {
		t.Fatalf("line2 %q: len = %d, want %d", line2, got, want)
	}
	capExact := len(line1) + 1 + len(line2)

	s, err := NewGraphiteStatsdSink(addr, "p", uint64(capExact))
	if err != nil {
		t.Fatalf("NewGraphiteStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := read(1)
	if len(got) != 1 {
		t.Fatalf("exact-boundary co-locate: got %d datagrams, want 1: %v", len(got), got)
	}
	gotLines := strings.Split(got[0], "\n")
	sameSet(t, gotLines, []string{line1, line2})
}

// TestGraphiteStatsdSink_BatchingExactBoundaryMinusOneSplits proves capExact
// (from the co-locate test above) was the TRUE boundary — one byte less and
// the same two lines must split into TWO separate single-line datagrams.
func TestGraphiteStatsdSink_BatchingExactBoundaryMinusOneSplits(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	reg.NewCounter("x").Add(1)
	reg.NewCounter("yy").Add(2)

	line1 := "p.x:1|c"
	line2 := "p.yy:2|c"
	capExact := len(line1) + 1 + len(line2)

	s, err := NewGraphiteStatsdSink(addr, "p", uint64(capExact-1))
	if err != nil {
		t.Fatalf("NewGraphiteStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := read(2)
	if len(got) != 2 {
		t.Fatalf("exact-boundary-minus-one: got %d datagrams, want 2: %v", len(got), got)
	}
	for _, dg := range got {
		if strings.Contains(dg, "\n") {
			t.Errorf("exact-boundary-minus-one: datagram %q unexpectedly contains an embedded newline", dg)
		}
	}
	sameSet(t, got, []string{line1, line2})
}

// TestGraphiteStatsdSink_BatchingOversizedLineAlone proves a single line
// whose own formatted length exceeds the cap is sent ALONE in its own
// oversized datagram, with NO truncation.
func TestGraphiteStatsdSink_BatchingOversizedLineAlone(t *testing.T) {
	addr, read := udpListener(t)

	longName := strings.Repeat("z", 40)
	reg := stats.NewRegistry()
	reg.NewCounter(longName).Add(1)
	reg.NewCounter("s").Add(1)

	oversizedLine := "p." + longName + ":1|c"
	shortLine := "p.s:1|c"
	if len(oversizedLine) <= 10 {
		t.Fatalf("test setup: oversizedLine %q (len %d) does not exceed the cap 10", oversizedLine, len(oversizedLine))
	}

	s, err := NewGraphiteStatsdSink(addr, "p", 10)
	if err != nil {
		t.Fatalf("NewGraphiteStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := read(2)
	if len(got) != 2 {
		t.Fatalf("oversized-alone: got %d datagrams, want 2: %v", len(got), got)
	}

	var foundOversized, foundShort bool
	for _, dg := range got {
		if strings.Contains(dg, "\n") {
			t.Errorf("oversized-alone: datagram %q unexpectedly contains an embedded newline", dg)
			continue
		}
		switch dg {
		case oversizedLine:
			foundOversized = true
			if len(dg) != len(oversizedLine) {
				t.Errorf("oversized-alone: oversized datagram len = %d, want %d (truncated?)", len(dg), len(oversizedLine))
			}
		case shortLine:
			foundShort = true
		default:
			t.Errorf("oversized-alone: unexpected datagram %q", dg)
		}
	}
	if !foundOversized {
		t.Errorf("oversized-alone: did not observe the oversized line %q alone in got %v", oversizedLine, got)
	}
	if !foundShort {
		t.Errorf("oversized-alone: did not observe the short line %q in got %v", shortLine, got)
	}
}

// TestGraphiteStatsdSink_BatchingCapZeroExplicitMatchesAbsent is a regression
// guard: an EXPLICIT cap of 0 must behave as one-line-per-datagram — no
// special-cased branch is needed for the degenerate zero-cap case.
func TestGraphiteStatsdSink_BatchingCapZeroExplicitMatchesAbsent(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	c := reg.NewCounter("cluster.backend.upstream_rq_total")
	c.Add(7)
	g := reg.NewGauge("cluster.backend.membership_total")
	g.Set(1)

	s, err := NewGraphiteStatsdSink(addr, "grpfx", 0)
	if err != nil {
		t.Fatalf("NewGraphiteStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := read(2)
	want := []string{
		"grpfx.cluster.upstream_rq_total;envoy.cluster_name=backend:7|c",
		"grpfx.cluster.membership_total;envoy.cluster_name=backend:1|g",
	}
	sameSet(t, got, want)
	for _, dg := range got {
		if strings.Contains(dg, "\n") {
			t.Errorf("cap=0 explicit: datagram %q unexpectedly contains an embedded newline", dg)
		}
	}
}

func TestGraphiteStatsdSink_EmptyBatch(t *testing.T) {
	addr, read := udpListener(t)

	s, err := NewGraphiteStatsdSink(addr, "p", 100)
	if err != nil {
		t.Fatalf("NewGraphiteStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Submit nil — no panic, no datagram.
	s.Submit(nil)
	got := read(1) // will block until deadline (500ms), then return 0
	if len(got) != 0 {
		t.Errorf("empty batch: got %d datagrams, want 0: %v", len(got), got)
	}
}

func TestGraphiteStatsdSink_CloseIdempotent(t *testing.T) {
	addr, _ := udpListener(t)

	s, err := NewGraphiteStatsdSink(addr, "p", 0)
	if err != nil {
		t.Fatalf("NewGraphiteStatsdSink: %v", err)
	}

	err1 := s.Close()
	err2 := s.Close()
	if err1 != nil {
		t.Errorf("Close() first call: %v", err1)
	}
	if err2 != nil {
		t.Errorf("Close() second call: %v", err2)
	}
}

func TestGraphiteStatsdSink_ResolveError(t *testing.T) {
	s, err := NewGraphiteStatsdSink("not-an-addr", "p", 0)
	if err == nil {
		_ = s.Close()
		t.Fatal("NewGraphiteStatsdSink with invalid addr: want error, got nil")
	}
	if s != nil {
		t.Errorf("NewGraphiteStatsdSink with invalid addr: want nil sink, got non-nil")
	}
	if err != nil && !strings.Contains(err.Error(), "graphite_statsd") {
		t.Errorf("NewGraphiteStatsdSink error %q does not mention graphite_statsd", err.Error())
	}
}
