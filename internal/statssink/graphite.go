package statssink

import (
	"fmt"
	"net"
	"strings"

	dto "github.com/prometheus/client_model/go"

	"github.com/pgdad/envoy-go/internal/stats"
)

// GraphiteStatsdSink writes the frozen registry snapshot as graphite-flavored
// statsd UDP datagrams every flush (ADR-0275): one line per metric family,
// <prefix>.<residual>[;key=value;…]:<value>|<c|g> — tags are folded INTO the
// metric NAME as ;k=v pairs (CONTRAST DogStatsdSink's trailing |#k:v suffix;
// live-pinned AMEND-GR-TAGFORMAT, SPEC-57 §11). COUNTER families carry the
// per-flush DELTA over a FOURTH sink-private always-on deltaState (zero-delta
// lines re-emitted every flush — AMEND-GR-DELTA); GAUGE families carry the
// ABSOLUTE value. Keys are the dotted envoy.<tag> form in ExtractTags's
// NATURAL (unsorted) slice order (AMEND-GR-TAGORDER — the reference does not
// sort either; its two-tag order is the REVERSE of ours, a documented
// cross-side coverage boundary). A tag-free name carries NO ';' at all.
// Batching reuses the shared appendBatchLine/flushBatch pair (ADR-0267
// semantics verbatim: strict-> prospective overflow incl. the +1 separator
// byte, \n-separated never terminated, oversized-sent-alone untruncated,
// 0/absent = one line per datagram; an EXPLICIT 0 never reaches here — the
// parse arm rejects it, PGV parity). SYNCHRONOUS (no goroutine): a THIRD
// independent connected *net.UDPConn via the shared udpWriter; envoy-go has
// no histograms (ADR-0060), so only |c/|g lines are produced.
type GraphiteStatsdSink struct {
	udpWriter
	prefix              string
	delta               *deltaState // always non-nil — a FOURTH sink-private instance, never shared
	maxBytesPerDatagram uint64      // 0 = one line per datagram (explicit 0 is parse-rejected upstream)
}

// NewGraphiteStatsdSink resolves udpAddr, dials a connected UDP socket (a THIRD
// independent *net.UDPConn), and returns a ready sink. A resolve/dial error is
// returned verbatim (-> main.go log.Fatalf, the statsd/dog_statsd precedent).
func NewGraphiteStatsdSink(udpAddr string, prefix string, maxBytesPerDatagram uint64) (*GraphiteStatsdSink, error) {
	raddr, err := net.ResolveUDPAddr("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("statssink: resolve graphite_statsd udp address %q: %w", udpAddr, err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, fmt.Errorf("statssink: dial graphite_statsd udp %q: %w", udpAddr, err)
	}
	return &GraphiteStatsdSink{
		udpWriter:           udpWriter{conn: conn, sinkLabel: "graphite_statsd"},
		prefix:              prefix,
		delta:               newDeltaState(),
		maxBytesPerDatagram: maxBytesPerDatagram,
	}, nil
}

// Submit applies the sink-private deltaState, then per family folds the
// ExtractTags residual + graphiteTagSuffix INTO the emitted NAME (the
// tag-suffix return stays "" — graphite's tags precede the ':'), accumulating
// lines into a per-call buffer via the shared batching pair. Called serially
// by the Flusher.
func (s *GraphiteStatsdSink) Submit(batch []*dto.MetricFamily) {
	batch = s.delta.apply(batch)
	var buf strings.Builder // a PER-CALL buffer — batching never spans across flushes
	emitStatsdLines(batch, func(fam *dto.MetricFamily) (string, string) {
		residual, labels, err := stats.ExtractTags(fam.GetName())
		if err != nil {
			// Defensive: can't happen for a registered name — full name, no tags.
			residual, labels = fam.GetName(), nil
		}
		return s.prefix + "." + residual + graphiteTagSuffix(labels), ""
	}, func(line string) {
		appendBatchLine(&buf, line, s.maxBytesPerDatagram, s.write)
	})
	flushBatch(&buf, s.write)
}

// graphiteTagSuffix renders labels as the graphite name-embedded tag suffix
// ";envoy.k1=v1;envoy.k2=v2" in the slice's NATURAL (unsorted) order — the ';'
// separates the name from the FIRST tag too, and an empty label set returns ""
// (no ';' anywhere — AMEND-GR-TAGFORMAT). Keys take the dotted envoy. form
// (the formatTagSuffix precedent). THE one novel piece of phase 57.
func graphiteTagSuffix(labels []stats.Label) string {
	if len(labels) == 0 {
		return ""
	}
	var b strings.Builder
	for _, l := range labels {
		b.WriteByte(';')
		b.WriteString("envoy.")
		b.WriteString(strings.TrimPrefix(l.Key, "envoy_"))
		b.WriteByte('=')
		b.WriteString(l.Value)
	}
	return b.String()
}
