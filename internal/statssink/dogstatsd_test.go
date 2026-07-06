package statssink

import (
	"strings"
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

// NOTE: udpListener(t) and sameSet(t, got, want) are defined in statsd_test.go
// (same package statssink) — reused directly here, not redefined.

func TestDogStatsdSink_CounterAndGaugePrefixJoin(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	c := reg.NewCounter("cluster.backend.upstream_rq_total")
	c.Add(7)
	g := reg.NewGauge("cluster.backend.membership_total")
	g.Set(1)

	s, err := NewDogStatsdSink(addr, "dsdpfx", 0)
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := read(2)
	want := []string{
		"dsdpfx.cluster.upstream_rq_total:7|c|#envoy.cluster_name:backend",
		"dsdpfx.cluster.membership_total:1|g|#envoy.cluster_name:backend",
	}
	sameSet(t, got, want)
}

func TestDogStatsdSink_SN4StatusClassNaturalTagOrder(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	rq2xx := reg.NewCounter("http.hcm_local.downstream_rq_2xx")
	rq2xx.Add(5)

	s, err := NewDogStatsdSink(addr, "dsdpfx", 0)
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := read(1)
	// Order matters here: response_code_class BEFORE http_conn_manager_prefix
	// (the SN4-prepend order) — a sort.Slice bug would emit
	// http_conn_manager_prefix first and this exact-literal assertion would
	// catch it (D-DSD-TAGS-ORDER).
	want := "dsdpfx.http.downstream_rq_xx:5|c|#envoy.response_code_class:2,envoy.http_conn_manager_prefix:hcm_local"
	if len(got) != 1 || got[0] != want {
		t.Errorf("SN4 natural tag order: got %v, want [%s]", got, want)
	}
}

func TestDogStatsdSink_UntaggedNoSuffix(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	u := reg.NewCounter("server.dynamic_unknown_fields")
	u.Add(0)

	s, err := NewDogStatsdSink(addr, "dsdpfx", 0)
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := read(1)
	want := "dsdpfx.server.dynamic_unknown_fields:0|c"
	if len(got) != 1 || got[0] != want {
		t.Errorf("untagged: got %v, want [%s]", got, want)
	}
}

func TestDogStatsdSink_DeltaSemanticsAcrossFlushes(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	c := reg.NewCounter("cluster.backend.upstream_rq_total")

	s, err := NewDogStatsdSink(addr, "p", 0)
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Flush 1: cumulative 7 → delta 7.
	c.Add(7)
	s.Submit(snapshot(reg, 0))
	got := read(1)
	want := "p.cluster.upstream_rq_total:7|c|#envoy.cluster_name:backend"
	if len(got) != 1 || got[0] != want {
		t.Errorf("flush1: got %v, want [%s]", got, want)
	}

	// Flush 2: no new increments → delta 0 (proves the sink-private deltaState
	// is live).
	s.Submit(snapshot(reg, 0))
	got = read(1)
	want = "p.cluster.upstream_rq_total:0|c|#envoy.cluster_name:backend"
	if len(got) != 1 || got[0] != want {
		t.Errorf("flush2 (idle): got %v, want [%s]", got, want)
	}

	// Flush 3: add 3 more (cumulative 10) → delta 3.
	c.Add(3)
	s.Submit(snapshot(reg, 0))
	got = read(1)
	want = "p.cluster.upstream_rq_total:3|c|#envoy.cluster_name:backend"
	if len(got) != 1 || got[0] != want {
		t.Errorf("flush3: got %v, want [%s]", got, want)
	}
}

func TestDogStatsdSink_IndependentFromStatsdSinkDelta(t *testing.T) {
	dsdAddr, dsdRead := udpListener(t)
	sdAddr, sdRead := udpListener(t)

	reg := stats.NewRegistry()
	c := reg.NewCounter("cluster.backend.upstream_rq_total")
	c.Add(7)

	dsd, err := NewDogStatsdSink(dsdAddr, "p", 0)
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = dsd.Close() })

	sd, err := NewStatsdSink(sdAddr, "p")
	if err != nil {
		t.Fatalf("NewStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = sd.Close() })

	batch := snapshot(reg, 0)
	dsd.Submit(batch)
	sd.Submit(batch)

	got := dsdRead(1)
	want := "p.cluster.upstream_rq_total:7|c|#envoy.cluster_name:backend"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("dsd flush1: got %v, want [%s]", got, want)
	}
	gotSd := sdRead(1)
	wantSd := "p.cluster.backend.upstream_rq_total:7|c"
	if len(gotSd) != 1 || gotSd[0] != wantSd {
		t.Fatalf("sd flush1: got %v, want [%s]", gotSd, wantSd)
	}

	// Flush the StatsdSink an EXTRA time in between — the DogStatsdSink's next
	// delta must be UNAFFECTED, proving no shared deltaState.
	sd.Submit(snapshot(reg, 0))
	_ = sdRead(1)

	c.Add(3)
	batch = snapshot(reg, 0)
	dsd.Submit(batch)
	got = dsdRead(1)
	want = "p.cluster.upstream_rq_total:3|c|#envoy.cluster_name:backend"
	if len(got) != 1 || got[0] != want {
		t.Errorf("dsd flush2 after extra sd flush: got %v, want [%s]", got, want)
	}
}

func TestDogStatsdSink_GaugeAbsoluteAcrossFlushes(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	g := reg.NewGauge("cluster.backend.membership_total")

	s, err := NewDogStatsdSink(addr, "p", 0)
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Flush 1: absolute 1.
	g.Set(1)
	s.Submit(snapshot(reg, 0))
	got := read(1)
	want := "p.cluster.membership_total:1|g|#envoy.cluster_name:backend"
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

func TestDogStatsdSink_NegativeGauge(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	g := reg.NewGauge("cluster.backend.some_signed_gauge")
	g.Set(-5)

	s, err := NewDogStatsdSink(addr, "p", 0)
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := read(1)
	want := "p.cluster.some_signed_gauge:-5|g|#envoy.cluster_name:backend"
	if len(got) != 1 || got[0] != want {
		t.Errorf("negative gauge: got %v, want [%s]", got, want)
	}
}

func TestDogStatsdSink_DefaultPrefix(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	c := reg.NewCounter("cluster.backend.upstream_rq_total")
	c.Add(1)

	s, err := NewDogStatsdSink(addr, "envoy", 0)
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := read(1)
	if len(got) != 1 {
		t.Fatalf("default prefix: got %d datagrams, want 1", len(got))
	}
	if len(got[0]) < 6 || got[0][:6] != "envoy." {
		t.Errorf("default prefix: datagram %q does not start with 'envoy.'", got[0])
	}
}

func TestDogStatsdSink_EmptyBatch(t *testing.T) {
	addr, read := udpListener(t)

	s, err := NewDogStatsdSink(addr, "p", 0)
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Submit nil — no panic, no datagram.
	s.Submit(nil)
	got := read(1) // will block until deadline (500ms), then return 0
	if len(got) != 0 {
		t.Errorf("empty batch: got %d datagrams, want 0: %v", len(got), got)
	}
}

func TestDogStatsdSink_CloseIdempotent(t *testing.T) {
	addr, _ := udpListener(t)

	s, err := NewDogStatsdSink(addr, "p", 0)
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
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

func TestDogStatsdSink_ResolveError(t *testing.T) {
	s, err := NewDogStatsdSink("not a valid addr", "p", 0)
	if err == nil {
		_ = s.Close()
		t.Fatal("NewDogStatsdSink with invalid addr: want error, got nil")
	}
	if s != nil {
		t.Errorf("NewDogStatsdSink with invalid addr: want nil sink, got non-nil")
	}
}

// TestDogStatsdSink_BatchingExactBoundaryColocates is the LOAD-BEARING
// boundary probe (mirrors the SPEC's live D-DSDB-BOUNDARY probe): a cap set
// to EXACTLY the combined length of two lines (line1 + "\n" + line2) must
// co-locate both lines into ONE datagram — the boundary is INCLUSIVE
// (`prospective > cap`, STRICT — a buffer landing EXACTLY at the cap after
// appending still fits).
func TestDogStatsdSink_BatchingExactBoundaryColocates(t *testing.T) {
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

	s, err := NewDogStatsdSink(addr, "p", uint64(capExact))
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
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

// TestDogStatsdSink_BatchingExactBoundaryMinusOneSplits proves capExact (from
// the co-locate test above) was the TRUE boundary — one byte less and the
// same two lines must split into TWO separate single-line datagrams.
func TestDogStatsdSink_BatchingExactBoundaryMinusOneSplits(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	reg.NewCounter("x").Add(1)
	reg.NewCounter("yy").Add(2)

	line1 := "p.x:1|c"
	line2 := "p.yy:2|c"
	capExact := len(line1) + 1 + len(line2)

	s, err := NewDogStatsdSink(addr, "p", uint64(capExact-1))
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
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

// TestDogStatsdSink_BatchingOversizedLineAlone proves a single line whose own
// formatted length exceeds the cap is sent ALONE in its own oversized
// datagram, with NO truncation and NO special-cased branch — it falls out of
// the general appendLine/flush algorithm.
func TestDogStatsdSink_BatchingOversizedLineAlone(t *testing.T) {
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

	s, err := NewDogStatsdSink(addr, "p", 10)
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
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

// TestDogStatsdSink_BatchingPreservesOrderAcrossFlushes proves the batching
// rewrite does not perturb the registry's own walk order: with a cap generous
// enough to co-locate all three lines into one datagram, the RELATIVE ORDER
// of lines observed in a first flush must recur identically in a second
// flush (mirrors AMEND-DSDB-JOIN-ORDER-CONFIRMED's live-probe methodology —
// order STABILITY across repeated flushes, not a hardcoded expected order).
func TestDogStatsdSink_BatchingPreservesOrderAcrossFlushes(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	c1 := reg.NewCounter("counter_one")
	c2 := reg.NewCounter("counter_two")
	c3 := reg.NewCounter("counter_three")
	c1.Add(1)
	c2.Add(2)
	c3.Add(3)

	s, err := NewDogStatsdSink(addr, "p", 100)
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	nameOf := func(line string) string {
		idx := strings.Index(line, ":")
		if idx < 0 {
			return line
		}
		return line[:idx]
	}

	s.Submit(snapshot(reg, 0))
	got1 := read(1)
	if len(got1) != 1 {
		t.Fatalf("flush1: got %d datagrams, want 1: %v", len(got1), got1)
	}
	lines1 := strings.Split(got1[0], "\n")
	if len(lines1) != 3 {
		t.Fatalf("flush1: got %d lines, want 3: %v", len(lines1), lines1)
	}
	var order1 []string
	for _, l := range lines1 {
		order1 = append(order1, nameOf(l))
	}

	// Second flush: no new increments (deltas will be 0), but the ORDER must
	// be identical.
	s.Submit(snapshot(reg, 0))
	got2 := read(1)
	if len(got2) != 1 {
		t.Fatalf("flush2: got %d datagrams, want 1: %v", len(got2), got2)
	}
	lines2 := strings.Split(got2[0], "\n")
	if len(lines2) != 3 {
		t.Fatalf("flush2: got %d lines, want 3: %v", len(lines2), lines2)
	}
	var order2 []string
	for _, l := range lines2 {
		order2 = append(order2, nameOf(l))
	}

	if len(order1) != len(order2) {
		t.Fatalf("order length mismatch: flush1 %v, flush2 %v", order1, order2)
	}
	for i := range order1 {
		if order1[i] != order2[i] {
			t.Errorf("order mismatch at index %d: flush1 %v, flush2 %v", i, order1, order2)
			break
		}
	}
}

// TestDogStatsdSink_BatchingCapZeroExplicitMatchesAbsent is a regression
// guard: an EXPLICIT cap of 0 must behave identically to the phase-49
// (pre-batching) one-line-per-datagram behavior — no special-cased branch is
// needed for the degenerate zero-cap case.
func TestDogStatsdSink_BatchingCapZeroExplicitMatchesAbsent(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	c := reg.NewCounter("cluster.backend.upstream_rq_total")
	c.Add(7)
	g := reg.NewGauge("cluster.backend.membership_total")
	g.Set(1)

	s, err := NewDogStatsdSink(addr, "dsdpfx", 0)
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := read(2)
	want := []string{
		"dsdpfx.cluster.upstream_rq_total:7|c|#envoy.cluster_name:backend",
		"dsdpfx.cluster.membership_total:1|g|#envoy.cluster_name:backend",
	}
	sameSet(t, got, want)
	for _, dg := range got {
		if strings.Contains(dg, "\n") {
			t.Errorf("cap=0 explicit: datagram %q unexpectedly contains an embedded newline", dg)
		}
	}
}

// TestDogStatsdSink_BatchingEmptyBatchWithCapSet proves the trailing flush on
// an empty buffer is a no-op: an empty batch with a non-zero cap set must not
// panic and must write no datagram.
func TestDogStatsdSink_BatchingEmptyBatchWithCapSet(t *testing.T) {
	addr, read := udpListener(t)

	s, err := NewDogStatsdSink(addr, "p", 100)
	if err != nil {
		t.Fatalf("NewDogStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(nil)
	got := read(1) // will block until deadline (500ms), then return 0
	if len(got) != 0 {
		t.Errorf("empty batch with cap set: got %d datagrams, want 0: %v", len(got), got)
	}
}
