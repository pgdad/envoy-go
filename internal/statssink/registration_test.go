package statssink

import (
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/stats"
)

// countMetrics walks the registry and returns the number of registered metrics.
func countMetrics(reg *stats.Registry) int {
	n := 0
	reg.Walk(func(stats.Metric) { n++ })
	return n
}

// TestNoNewStat_RegistrationGuard pins D-MS-STATS-FINAL: the metrics_service sink
// surface delta is +0. Constructing a MetricsServiceSink + Flusher against a fresh
// registry must register ZERO new metrics — the reference registers no
// metrics_service-scoped stat (AMEND-MS-NO-SELF-STATS), and a self-counter
// registered pre-Freeze would itself appear in its own next flush. Channel-full
// drops are rate-limit-LOGGED, not counted. This proves no statssink/
// metrics_service-scoped name is added to the stat surface (stays 1200 / 1196).
func TestNoNewStat_RegistrationGuard(t *testing.T) {
	reg := stats.NewRegistry()
	if before := countMetrics(reg); before != 0 {
		t.Fatalf("fresh registry should have 0 metrics, got %d", before)
	}

	// The exact main.go construction shape: a MetricsServiceSink over a client +
	// node, then a Flusher over the registry/interval/sinks.
	client := &fakeMetricsClient{streams: []*fakeMetricsStream{{}}}
	sink := NewMetricsServiceSink(client, testNode(), false, false)
	t.Cleanup(func() { _ = sink.Close() })

	flusher := NewFlusher(reg, 500*time.Millisecond, []Sink{sink})
	_ = flusher

	if after := countMetrics(reg); after != 0 {
		t.Fatalf("sink+flusher constructors registered %d metric(s); D-MS-STATS-FINAL requires +0", after)
	}
}

// TestNoNewStat_StatsdRegistrationGuard pins D-SD-STATS-FINAL: the statsd UDP sink
// surface delta is +0. Constructing a StatsdSink + Flusher against a fresh registry
// must register ZERO new metrics — the reference registers no statsd-scoped stat
// and dials no sink cluster (so no incidental upstream_cx_*), and UDP write drops
// are rate-limit-LOGGED, not counted. This proves no statsd-scoped name is added to
// the stat surface (stays 1200 / non-H2 1196). net.DialUDP needs no live listener
// (UDP is connectionless), so a loopback address suffices.
func TestNoNewStat_StatsdRegistrationGuard(t *testing.T) {
	reg := stats.NewRegistry()
	if before := countMetrics(reg); before != 0 {
		t.Fatalf("fresh registry should have 0 metrics, got %d", before)
	}

	// The exact main.go construction shape: a StatsdSink over a UDP address, then a
	// Flusher over the registry/interval/sinks.
	sink, err := NewStatsdSink("127.0.0.1:65535", "envoy")
	if err != nil {
		t.Fatalf("NewStatsdSink: %v", err)
	}
	t.Cleanup(func() { _ = sink.Close() })

	flusher := NewFlusher(reg, 500*time.Millisecond, []Sink{sink})
	_ = flusher

	if after := countMetrics(reg); after != 0 {
		t.Fatalf("statsd sink+flusher constructors registered %d metric(s); D-SD-STATS-FINAL requires +0", after)
	}
}
