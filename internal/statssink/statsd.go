package statssink

import (
	"fmt"
	"net"

	dto "github.com/prometheus/client_model/go"
)

// StatsdSink writes the frozen registry snapshot to a statsd server as UDP
// datagrams every flush (ADR-0265): one statsd line per metric family,
// <prefix>.<dotted-name>:<value>|<type>. COUNTER families carry the per-flush
// DELTA (|c) over a sink-private always-on deltaState (the canonical statsd
// increment — D-SD-DELTA; reusing the landed delta.go transform VERBATIM, no knob);
// GAUGE families carry the ABSOLUTE value (|g). The name is the full dotted
// internal name with tags inlined and ZERO labels (the StatsdSink proto does not
// support tagged metrics). envoy-go has no histograms (ADR-0060), so only |c/|g
// lines are produced (the reference's |ms timers have no analog). The line
// formatting, UDP write, and Close are the udpWriter + emitStatsdLines helpers
// shared with DogStatsdSink (udp.go).
//
// Writer shape (D-SD-LIFECYCLE-SHAPE): SYNCHRONOUS. A UDP Write is fire-and-forget
// (never blocks on a peer), and the Flusher calls Submit serially from its single
// goroutine, so Submit writes each datagram inline — no bounded channel, no writer
// goroutine (contrast MetricsServiceSink, whose channel absorbs a slow gRPC
// stream). This adds NO background mutator. A Write error is rate-limit-LOGGED and
// DROPPED (UDP is lossy by design; NOT counted — +0 self-stats, D-SD-STATS-FINAL).
type StatsdSink struct {
	udpWriter
	prefix string
	delta  *deltaState // always non-nil — statsd |c is intrinsically a per-flush delta (no knob)
}

// NewStatsdSink resolves udpAddr (a host:port literal), dials a connected UDP
// socket (the FIRST *net.UDPConn in the tree), and returns a ready sink. A
// resolve/dial error is returned verbatim (-> main.go log.Fatalf, the
// metrics_service-client-error precedent).
func NewStatsdSink(udpAddr string, prefix string) (*StatsdSink, error) {
	raddr, err := net.ResolveUDPAddr("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("statssink: resolve statsd udp address %q: %w", udpAddr, err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, fmt.Errorf("statssink: dial statsd udp %q: %w", udpAddr, err)
	}
	return &StatsdSink{
		udpWriter: udpWriter{conn: conn, sinkLabel: "statsd"},
		prefix:    prefix,
		delta:     newDeltaState(),
	}, nil
}

// Submit applies the sink-private deltaState (COUNTER -> per-flush delta, GAUGE/
// other -> absolute pass-through; builds the sink's OWN batch, never mutates the
// shared snapshot slice) then writes one UDP datagram per family (the shared
// emitStatsdLines skeleton: full prefixed dotted name, no tag suffix). Called
// serially by the Flusher.
func (s *StatsdSink) Submit(batch []*dto.MetricFamily) {
	batch = s.delta.apply(batch)
	emitStatsdLines(batch, func(fam *dto.MetricFamily) (string, string) {
		return s.prefix + "." + fam.GetName(), ""
	}, s.write)
}
