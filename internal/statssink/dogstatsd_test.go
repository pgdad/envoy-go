package statssink

import (
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
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

	s, err := NewDogStatsdSink(addr, "dsdpfx")
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

	s, err := NewDogStatsdSink(addr, "dsdpfx")
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

	s, err := NewDogStatsdSink(addr, "dsdpfx")
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

	s, err := NewDogStatsdSink(addr, "p")
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

	dsd, err := NewDogStatsdSink(dsdAddr, "p")
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

	s, err := NewDogStatsdSink(addr, "p")
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

	s, err := NewDogStatsdSink(addr, "p")
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

	s, err := NewDogStatsdSink(addr, "envoy")
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

	s, err := NewDogStatsdSink(addr, "p")
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

	s, err := NewDogStatsdSink(addr, "p")
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
	s, err := NewDogStatsdSink("not a valid addr", "p")
	if err == nil {
		_ = s.Close()
		t.Fatal("NewDogStatsdSink with invalid addr: want error, got nil")
	}
	if s != nil {
		t.Errorf("NewDogStatsdSink with invalid addr: want nil sink, got non-nil")
	}
}
