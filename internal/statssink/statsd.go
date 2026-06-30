package statssink

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

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
// lines are produced (the reference's |ms timers have no analog).
//
// Writer shape (D-SD-LIFECYCLE-SHAPE): SYNCHRONOUS. A UDP Write is fire-and-forget
// (never blocks on a peer), and the Flusher calls Submit serially from its single
// goroutine, so Submit writes each datagram inline — no bounded channel, no writer
// goroutine (contrast MetricsServiceSink, whose channel absorbs a slow gRPC
// stream). This adds NO background mutator. A Write error is rate-limit-LOGGED and
// DROPPED (UDP is lossy by design; NOT counted — +0 self-stats, D-SD-STATS-FINAL).
type StatsdSink struct {
	conn   *net.UDPConn
	prefix string
	delta  *deltaState // always non-nil — statsd |c is intrinsically a per-flush delta (no knob)

	closeOnce   sync.Once
	closeErr    error
	lastDropLog atomic.Int64
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
	return &StatsdSink{conn: conn, prefix: prefix, delta: newDeltaState()}, nil
}

// Submit applies the sink-private deltaState (COUNTER -> per-flush delta, GAUGE/
// other -> absolute pass-through; builds the sink's OWN batch, never mutates the
// shared snapshot slice) then writes one UDP datagram per family. Called serially
// by the Flusher.
func (s *StatsdSink) Submit(batch []*dto.MetricFamily) {
	batch = s.delta.apply(batch)
	for _, fam := range batch {
		name := s.prefix + "." + fam.GetName()
		var suffix string
		switch fam.GetType() {
		case dto.MetricType_COUNTER:
			suffix = "|c"
		case dto.MetricType_GAUGE:
			suffix = "|g"
		default:
			continue // no other family type exists (no histograms — ADR-0060)
		}
		for _, m := range fam.GetMetric() {
			var v float64
			if fam.GetType() == dto.MetricType_GAUGE {
				v = m.GetGauge().GetValue()
			} else {
				v = m.GetCounter().GetValue()
			}
			line := name + ":" + strconv.FormatInt(int64(v), 10) + suffix
			s.write(line)
		}
	}
}

// write sends one statsd line as one UDP datagram. A Write error is rate-limit-
// logged (at most once per second — the accesslog lastDropLog idiom) and dropped.
func (s *StatsdSink) write(line string) {
	if _, err := s.conn.Write([]byte(line)); err != nil {
		now := time.Now().UnixNano()
		last := s.lastDropLog.Load()
		if now-last >= dropLogIntervalNanos && s.lastDropLog.CompareAndSwap(last, now) {
			log.Printf("statssink: statsd udp write failed, dropping line: %v", err)
		}
	}
}

// Close closes the UDP socket. Idempotent via sync.Once.
func (s *StatsdSink) Close() error {
	s.closeOnce.Do(func() {
		if s.conn != nil {
			s.closeErr = s.conn.Close()
		}
	})
	return s.closeErr
}
