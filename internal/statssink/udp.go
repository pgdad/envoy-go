package statssink

import (
	"log"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	dto "github.com/prometheus/client_model/go"
)

// udpWriter is the shared UDP-datagram writer embedded by StatsdSink and
// DogStatsdSink: a connected *net.UDPConn, the rate-limited write-error
// diagnostic, and the idempotent Close. sinkLabel is the log-line discriminator
// ("statsd" / "dog_statsd") so each sink keeps its exact diagnostic text.
type udpWriter struct {
	conn      *net.UDPConn
	sinkLabel string

	closeOnce   sync.Once
	closeErr    error
	lastDropLog atomic.Int64
}

// write sends one line (or one accumulated batch) as one UDP datagram. A Write
// error is rate-limit-logged (at most once per second — the accesslog
// lastDropLog idiom) and dropped.
func (w *udpWriter) write(line string) {
	if _, err := w.conn.Write([]byte(line)); err != nil {
		now := time.Now().UnixNano()
		last := w.lastDropLog.Load()
		if now-last >= dropLogIntervalNanos && w.lastDropLog.CompareAndSwap(last, now) {
			log.Printf("statssink: %s udp write failed, dropping line: %v", w.sinkLabel, err)
		}
	}
}

// Close closes the UDP socket. Idempotent via sync.Once.
func (w *udpWriter) Close() error {
	w.closeOnce.Do(func() {
		if w.conn != nil {
			w.closeErr = w.conn.Close()
		}
	})
	return w.closeErr
}

// emitStatsdLines walks a (delta-applied) batch and invokes emit with each
// formatted statsd metric line <name>:<value><suffix>[<tagSuffix>] — the Submit
// skeleton shared by StatsdSink and DogStatsdSink. COUNTER families carry |c and
// the (already-delta'd) counter value; GAUGE families carry |g and the absolute
// gauge value. nameAndTags maps a family to its emitted name and tag suffix
// (StatsdSink: prefixed full dotted name, no tags; DogStatsdSink: prefixed
// tag-residual name plus the |# suffix). envoy-go has no histograms (ADR-0060),
// so any other family type is skipped.
func emitStatsdLines(batch []*dto.MetricFamily, nameAndTags func(fam *dto.MetricFamily) (name, tagSuffix string), emit func(line string)) {
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
		name, tagSuffix := nameAndTags(fam)
		for _, m := range fam.GetMetric() {
			var v float64
			if fam.GetType() == dto.MetricType_GAUGE {
				v = m.GetGauge().GetValue()
			} else {
				v = m.GetCounter().GetValue()
			}
			emit(name + ":" + strconv.FormatInt(int64(v), 10) + suffix + tagSuffix)
		}
	}
}
