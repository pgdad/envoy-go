package statssink

import (
	"net"
	"sort"
	"testing"
	"time"

	"github.com/pgdad/envoy-go/internal/stats"
)

// udpListener spins a real UDP listener on 127.0.0.1:0 and returns its addr
// and a read helper that reads n datagrams (each one statsd line) with a short
// read deadline. The listener is closed at test cleanup.
func udpListener(t *testing.T) (addr string, read func(n int) []string) {
	t.Helper()
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("udpListener: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })

	addr = pc.LocalAddr().String()
	read = func(n int) []string {
		var lines []string
		buf := make([]byte, 4096)
		for i := 0; i < n; i++ {
			if err := pc.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
				t.Fatalf("SetReadDeadline: %v", err)
			}
			nn, _, err := pc.ReadFromUDP(buf)
			if err != nil {
				// read deadline elapsed — fewer datagrams arrived than expected
				break
			}
			lines = append(lines, string(buf[:nn]))
		}
		return lines
	}
	return addr, read
}

// sameSet asserts that got and want contain the same strings (order-independent).
func sameSet(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("datagram count: got %d, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
		return
	}
	g := make([]string, len(got))
	w := make([]string, len(want))
	copy(g, got)
	copy(w, want)
	sort.Strings(g)
	sort.Strings(w)
	for i := range g {
		if g[i] != w[i] {
			t.Errorf("datagram[%d]: got %q, want %q", i, g[i], w[i])
		}
	}
}

func TestStatsdSink_CounterAndGaugePrefixJoin(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	c := reg.NewCounter("cluster.backend.upstream_rq_total")
	c.Add(7)
	g := reg.NewGauge("cluster.backend.membership_healthy")
	g.Set(1)

	s, err := NewStatsdSink(addr, "myprefix")
	if err != nil {
		t.Fatalf("NewStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := read(2)
	want := []string{
		"myprefix.cluster.backend.upstream_rq_total:7|c",
		"myprefix.cluster.backend.membership_healthy:1|g",
	}
	sameSet(t, got, want)
}

func TestStatsdSink_DeltaSemanticsAcrossFlushes(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	c := reg.NewCounter("cluster.backend.upstream_rq_total")

	s, err := NewStatsdSink(addr, "p")
	if err != nil {
		t.Fatalf("NewStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Flush 1: cumulative 7 → delta 7.
	c.Add(7)
	s.Submit(snapshot(reg, 0))
	got := read(1)
	if len(got) != 1 || got[0] != "p.cluster.backend.upstream_rq_total:7|c" {
		t.Errorf("flush1: got %v, want [p.cluster.backend.upstream_rq_total:7|c]", got)
	}

	// Flush 2: no new increments → delta 0.
	s.Submit(snapshot(reg, 0))
	got = read(1)
	if len(got) != 1 || got[0] != "p.cluster.backend.upstream_rq_total:0|c" {
		t.Errorf("flush2 (idle): got %v, want [p.cluster.backend.upstream_rq_total:0|c]", got)
	}

	// Flush 3: add 3 more (cumulative 10) → delta 3.
	c.Add(3)
	s.Submit(snapshot(reg, 0))
	got = read(1)
	if len(got) != 1 || got[0] != "p.cluster.backend.upstream_rq_total:3|c" {
		t.Errorf("flush3: got %v, want [p.cluster.backend.upstream_rq_total:3|c]", got)
	}
}

func TestStatsdSink_GaugeAbsoluteAcrossFlushes(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	g := reg.NewGauge("cluster.backend.membership_healthy")

	s, err := NewStatsdSink(addr, "p")
	if err != nil {
		t.Fatalf("NewStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Flush 1: absolute 1.
	g.Set(1)
	s.Submit(snapshot(reg, 0))
	got := read(1)
	if len(got) != 1 || got[0] != "p.cluster.backend.membership_healthy:1|g" {
		t.Errorf("flush1: got %v, want [p.cluster.backend.membership_healthy:1|g]", got)
	}

	// Flush 2: same value; must emit 1|g (absolute), NOT a 0 delta.
	g.Set(1)
	s.Submit(snapshot(reg, 0))
	got = read(1)
	if len(got) != 1 || got[0] != "p.cluster.backend.membership_healthy:1|g" {
		t.Errorf("flush2 (same value): got %v, want [p.cluster.backend.membership_healthy:1|g]", got)
	}
}

func TestStatsdSink_NegativeGauge(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	g := reg.NewGauge("cluster.backend.some_signed_gauge")
	g.Set(-5)

	s, err := NewStatsdSink(addr, "p")
	if err != nil {
		t.Fatalf("NewStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	s.Submit(snapshot(reg, 0))
	got := read(1)
	if len(got) != 1 || got[0] != "p.cluster.backend.some_signed_gauge:-5|g" {
		t.Errorf("negative gauge: got %v, want [p.cluster.backend.some_signed_gauge:-5|g]", got)
	}
}

func TestStatsdSink_DefaultPrefix(t *testing.T) {
	addr, read := udpListener(t)

	reg := stats.NewRegistry()
	c := reg.NewCounter("cluster.backend.upstream_rq_total")
	c.Add(1)

	s, err := NewStatsdSink(addr, "envoy")
	if err != nil {
		t.Fatalf("NewStatsdSink: %v", err)
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

func TestStatsdSink_EmptyBatch(t *testing.T) {
	addr, read := udpListener(t)

	s, err := NewStatsdSink(addr, "p")
	if err != nil {
		t.Fatalf("NewStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Submit nil — no panic, no datagram.
	s.Submit(nil)
	got := read(1) // will block until deadline (500ms), then return 0
	if len(got) != 0 {
		t.Errorf("empty batch: got %d datagrams, want 0: %v", len(got), got)
	}
}

func TestStatsdSink_CloseIdempotent(t *testing.T) {
	addr, _ := udpListener(t)

	s, err := NewStatsdSink(addr, "p")
	if err != nil {
		t.Fatalf("NewStatsdSink: %v", err)
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

func TestStatsdSink_ResolveError(t *testing.T) {
	s, err := NewStatsdSink("not a valid addr", "p")
	if err == nil {
		_ = s.Close()
		t.Fatal("NewStatsdSink with invalid addr: want error, got nil")
	}
	if s != nil {
		t.Errorf("NewStatsdSink with invalid addr: want nil sink, got non-nil")
	}
}
