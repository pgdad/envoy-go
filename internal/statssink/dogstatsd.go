package statssink

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/esalaine/envoy-go/internal/stats"
)

// DogStatsdSink writes the frozen registry snapshot to a DogStatsd server as UDP
// datagrams every flush (ADR-0266): one DogStatsd line per metric family,
// <prefix>.<residual-name>:<value>|<type>[|#tag1:val1,...]. COUNTER families
// carry the per-flush DELTA (|c) over a SECOND, INDEPENDENT sink-private
// always-on deltaState (D-DSD-DELTA — reusing the landed delta.go transform
// VERBATIM, no knob, NEVER shared with StatsdSink's own instance); GAUGE
// families carry the ABSOLUTE value (|g). The name is stats.ExtractTags's
// residual dotted name (the SAME SN1-SN9+SN4 matcher label.go's labelMapper
// calls, consumed DIRECTLY here — not via labelMapper/LabelPair, since the
// target shape is an inline wire-string suffix, not a structured field); the
// extracted tags are formatted envoy.<key>:<value>, comma-joined, in the
// SLICE'S NATURAL (unsorted) order (D-DSD-TAGS-ORDER — the reference does NOT
// alphabetically sort; ExtractTags's own SN4-prepended order already matches
// it. CONTRAST labelMapper, which sorts because LabelPair order is immaterial
// to a structured Prometheus label — a DogStatsd tag suffix is a literal wire
// string where order is part of the byte-format). A name with zero extracted
// tags (or an ExtractTags error — defensive, can't happen for a registered
// name) emits with NO |# suffix at all. envoy-go has no histograms (ADR-0060),
// so only |c/|g lines are produced.
//
// Writer shape (D-DSD-LIFECYCLE): SYNCHRONOUS, identical to StatsdSink — a UDP
// Write is fire-and-forget and the Flusher calls Submit serially, so Submit
// writes each datagram inline. This is a SECOND, INDEPENDENT *net.UDPConn from
// StatsdSink's (never shared).
type DogStatsdSink struct {
	conn   *net.UDPConn
	prefix string
	delta  *deltaState // always non-nil — a SECOND, independent instance from StatsdSink's

	closeOnce   sync.Once
	closeErr    error
	lastDropLog atomic.Int64
}

// NewDogStatsdSink resolves udpAddr (a host:port literal), dials a connected UDP
// socket (a SECOND *net.UDPConn in the tree, independent of any StatsdSink's),
// and returns a ready sink. A resolve/dial error is returned verbatim (->
// main.go log.Fatalf, the StatsdSink-error precedent).
func NewDogStatsdSink(udpAddr string, prefix string) (*DogStatsdSink, error) {
	raddr, err := net.ResolveUDPAddr("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("statssink: resolve dog_statsd udp address %q: %w", udpAddr, err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, fmt.Errorf("statssink: dial dog_statsd udp %q: %w", udpAddr, err)
	}
	return &DogStatsdSink{conn: conn, prefix: prefix, delta: newDeltaState()}, nil
}

// Submit applies the sink-private deltaState (COUNTER -> per-flush delta, GAUGE/
// other -> absolute pass-through; builds the sink's OWN batch, never mutates the
// shared snapshot slice) then, per family, extracts the residual name + tags via
// stats.ExtractTags and writes ONE UDP datagram. Called serially by the Flusher.
func (s *DogStatsdSink) Submit(batch []*dto.MetricFamily) {
	batch = s.delta.apply(batch)
	for _, fam := range batch {
		var suffix string
		switch fam.GetType() {
		case dto.MetricType_COUNTER:
			suffix = "|c"
		case dto.MetricType_GAUGE:
			suffix = "|g"
		default:
			continue // no other family type exists (no histograms — ADR-0060)
		}
		residual, labels, err := stats.ExtractTags(fam.GetName())
		if err != nil {
			// Defensive: can't happen for a registered name (the label.go labelMapper
			// precedent) — fall back to the full untransformed name, no tags.
			residual, labels = fam.GetName(), nil
		}
		name := s.prefix + "." + residual
		tagSuffix := formatTagSuffix(labels)
		for _, m := range fam.GetMetric() {
			var v float64
			if fam.GetType() == dto.MetricType_GAUGE {
				v = m.GetGauge().GetValue()
			} else {
				v = m.GetCounter().GetValue()
			}
			line := name + ":" + strconv.FormatInt(int64(v), 10) + suffix + tagSuffix
			s.write(line)
		}
	}
}

// formatTagSuffix builds the inline "|#tag1:val1,tag2:val2" suffix from labels
// IN THEIR NATURAL (unsorted) ORDER — no sort.Slice (D-DSD-TAGS-ORDER; CONTRAST
// labelMapper.apply in label.go, which sorts because LabelPair order is
// immaterial to a structured Prometheus label, unlike this literal wire string).
// Returns "" when labels is empty (no |# suffix at all — AMEND-DSD-NAME-CONFIRMED).
func formatTagSuffix(labels []stats.Label) string {
	if len(labels) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("|#")
	for i, l := range labels {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("envoy.")
		b.WriteString(strings.TrimPrefix(l.Key, "envoy_"))
		b.WriteByte(':')
		b.WriteString(l.Value)
	}
	return b.String()
}

// write sends one DogStatsd line as one UDP datagram. A Write error is
// rate-limit-logged (at most once per second — the accesslog lastDropLog idiom)
// and dropped.
func (s *DogStatsdSink) write(line string) {
	if _, err := s.conn.Write([]byte(line)); err != nil {
		now := time.Now().UnixNano()
		last := s.lastDropLog.Load()
		if now-last >= dropLogIntervalNanos && s.lastDropLog.CompareAndSwap(last, now) {
			log.Printf("statssink: dog_statsd udp write failed, dropping line: %v", err)
		}
	}
}

// Close closes the UDP socket. Idempotent via sync.Once.
func (s *DogStatsdSink) Close() error {
	s.closeOnce.Do(func() {
		if s.conn != nil {
			s.closeErr = s.conn.Close()
		}
	})
	return s.closeErr
}
