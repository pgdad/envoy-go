package statssink

import (
	"fmt"
	"net"
	"strings"

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
// appendBatchLine (udp.go).
//
// Writer shape (D-DSD-LIFECYCLE): SYNCHRONOUS, identical to StatsdSink — a UDP
// Write is fire-and-forget and the Flusher calls Submit serially, so Submit
// writes each datagram inline. This is a SECOND, INDEPENDENT *net.UDPConn from
// StatsdSink's (never shared). The line formatting, UDP write, and Close are the
// udpWriter + emitStatsdLines helpers shared with StatsdSink (udp.go).
type DogStatsdSink struct {
	udpWriter
	prefix              string
	delta               *deltaState // always non-nil — a SECOND, independent instance from StatsdSink's
	maxBytesPerDatagram uint64      // NEW (ADR-0267): 0 means "one metric per datagram" (phase-49 behavior)
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
	return &DogStatsdSink{
		udpWriter:           udpWriter{conn: conn, sinkLabel: "dog_statsd"},
		prefix:              prefix,
		delta:               newDeltaState(),
		maxBytesPerDatagram: maxBytesPerDatagram,
	}, nil
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
	emitStatsdLines(batch, func(fam *dto.MetricFamily) (string, string) {
		residual, labels, err := stats.ExtractTags(fam.GetName())
		if err != nil {
			// Defensive: can't happen for a registered name (the label.go labelMapper
			// precedent) — fall back to the full untransformed name, no tags.
			residual, labels = fam.GetName(), nil
		}
		return s.prefix + "." + residual, formatTagSuffix(labels)
	}, func(line string) {
		appendBatchLine(&buf, line, s.maxBytesPerDatagram, s.write)
	})
	flushBatch(&buf, s.write)
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
