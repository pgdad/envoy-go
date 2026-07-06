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

	"github.com/pgdad/envoy-go/internal/stats"
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
// Batching (ADR-0267): each already-formatted line is accumulated into a
// PER-CALL strings.Builder buffer (batching never spans across flushes) via
// appendLine, which flushes the buffer as its own UDP datagram FIRST whenever
// appending the next line would make it STRICTLY exceed maxBytesPerDatagram
// (a buffer landing EXACTLY at the cap after appending still fits — the
// boundary is INCLUSIVE). A single line whose own formatted length exceeds the
// cap is sent alone in its own oversized datagram, with NO special-cased
// branch: an empty buffer always accepts the first line unconditionally, so
// an oversized line is accepted, then forces the very next line (or the
// trailing flush, if it's last) to flush it alone. maxBytesPerDatagram == 0
// (absent field, or an explicit degenerate zero) needs no special case
// either — it reproduces the phase-49 one-line-per-datagram behavior exactly,
// since every append past the first in an already-non-empty buffer always
// exceeds a cap of 0.
//
// Writer shape (D-DSD-LIFECYCLE): SYNCHRONOUS, identical to StatsdSink — a UDP
// Write is fire-and-forget and the Flusher calls Submit serially, so Submit
// writes each datagram inline. This is a SECOND, INDEPENDENT *net.UDPConn from
// StatsdSink's (never shared).
type DogStatsdSink struct {
	conn                *net.UDPConn
	prefix              string
	delta               *deltaState // always non-nil — a SECOND, independent instance from StatsdSink's
	maxBytesPerDatagram uint64      // NEW (ADR-0267): 0 means "one metric per datagram" (phase-49 behavior)

	closeOnce   sync.Once
	closeErr    error
	lastDropLog atomic.Int64
}

// NewDogStatsdSink resolves udpAddr (a host:port literal), dials a connected UDP
// socket (a SECOND *net.UDPConn in the tree, independent of any StatsdSink's),
// and returns a ready sink. A resolve/dial error is returned verbatim (->
// main.go log.Fatalf, the StatsdSink-error precedent). maxBytesPerDatagram
// configures the batching cap (ADR-0267); 0 means "one metric per datagram"
// (phase-49 behavior, unchanged).
func NewDogStatsdSink(udpAddr string, prefix string, maxBytesPerDatagram uint64) (*DogStatsdSink, error) {
	raddr, err := net.ResolveUDPAddr("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("statssink: resolve dog_statsd udp address %q: %w", udpAddr, err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, fmt.Errorf("statssink: dial dog_statsd udp %q: %w", udpAddr, err)
	}
	return &DogStatsdSink{conn: conn, prefix: prefix, delta: newDeltaState(), maxBytesPerDatagram: maxBytesPerDatagram}, nil
}

// Submit applies the sink-private deltaState (COUNTER -> per-flush delta, GAUGE/
// other -> absolute pass-through; builds the sink's OWN batch, never mutates the
// shared snapshot slice) then, per family, extracts the residual name + tags via
// stats.ExtractTags and accumulates each line into a per-call buffer, flushing
// it as one or more UDP datagrams per the batching cap (ADR-0267). Called
// serially by the Flusher.
func (s *DogStatsdSink) Submit(batch []*dto.MetricFamily) {
	batch = s.delta.apply(batch)
	var buf strings.Builder // a PER-CALL buffer — batching never spans across flushes
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
			s.appendLine(&buf, line) // REPLACES the phase-49 s.write(line) call site
		}
	}
	s.flush(&buf) // flush any remaining partial buffer at the end of the batch
}

// appendLine accumulates line into buf, flushing buf as its own datagram FIRST
// if appending would make it STRICTLY exceed maxBytesPerDatagram (the
// comparison is `>`, NOT `>=` — a buffer that lands EXACTLY at the cap after
// appending still fits, live-proven AMEND-DSDB-BOUNDARY-CONFIRMED). When buf is
// EMPTY, the line is ALWAYS accepted unconditionally — even if the line alone
// exceeds the cap — which is what makes an oversized single line "sent alone"
// fall out of the SAME general algorithm with NO special-cased branch: on the
// NEXT call, buf.Len() already exceeds the cap, so ANY next line's prospective
// size trivially exceeds the cap too, forcing a flush of the oversized line
// alone before the next line is added; if the oversized line is the LAST in
// the batch, Submit's trailing flush sends it. maxBytesPerDatagram == 0
// (absent field or an explicit degenerate zero) needs NO special case either:
// an empty buf always accepts the first line unconditionally, then the NEXT
// append's prospective size (>= 1) is never <= 0, so every line flushes alone
// before the next is added — reproducing phase 49's one-line-per-datagram
// behavior exactly.
func (s *DogStatsdSink) appendLine(buf *strings.Builder, line string) {
	if buf.Len() == 0 {
		buf.WriteString(line)
		return
	}
	prospective := uint64(buf.Len()) + 1 + uint64(len(line)) // +1 for the "\n" separator
	if prospective > s.maxBytesPerDatagram {
		s.flush(buf)
		buf.WriteString(line)
		return
	}
	buf.WriteByte('\n')
	buf.WriteString(line)
}

// flush writes buf's contents as ONE UDP datagram (if non-empty) and resets it.
func (s *DogStatsdSink) flush(buf *strings.Builder) {
	if buf.Len() == 0 {
		return
	}
	s.write(buf.String()) // the EXISTING write() — rate-limit-logged-and-dropped on error, UNCHANGED
	buf.Reset()
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
