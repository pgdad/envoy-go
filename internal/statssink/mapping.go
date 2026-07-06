// Package statssink is envoy-go's periodic gRPC metrics_service stats sink: it
// snapshots the frozen process-global stats.Registry every stats_flush_interval
// and streams it as io.prometheus.client.MetricFamily protos over the
// MetricsService.StreamMetrics client-streaming RPC (ADR-0262). The mapping is
// cumulative/no-labels — each Counter becomes a COUNTER family and each Gauge a
// GAUGE family under its full dotted internal name.
package statssink

import (
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"

	"github.com/pgdad/envoy-go/internal/stats"
)

// snapshot walks the frozen registry and maps each Counter/Gauge to a
// cumulative/no-labels io.prometheus.client.MetricFamily (the metrics_service
// default mapping, ADR-0262): the FULL dotted Name() (tags inlined), the
// absolute Load() value, ZERO LabelPairs (emit_tags_as_labels=false), a
// flush-time TimestampMs, no Help. Histograms have no envoy-go analog
// (ADR-0060), so only COUNTER/GAUGE families are produced.
func snapshot(reg *stats.Registry, nowMs int64) []*dto.MetricFamily {
	var out []*dto.MetricFamily
	reg.Walk(func(m stats.Metric) {
		switch v := m.(type) {
		case *stats.Counter:
			out = append(out, &dto.MetricFamily{
				Name: proto.String(v.Name()),
				Type: dto.MetricType_COUNTER.Enum(),
				Metric: []*dto.Metric{{
					Counter:     &dto.Counter{Value: proto.Float64(float64(v.Load()))},
					TimestampMs: proto.Int64(nowMs),
				}},
			})
		case *stats.Gauge:
			out = append(out, &dto.MetricFamily{
				Name: proto.String(v.Name()),
				Type: dto.MetricType_GAUGE.Enum(),
				Metric: []*dto.Metric{{
					Gauge:       &dto.Gauge{Value: proto.Float64(float64(v.Load()))},
					TimestampMs: proto.Int64(nowMs),
				}},
			})
		}
	})
	return out
}
