package statssink

import (
	"sort"
	"strings"

	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"

	"github.com/pgdad/envoy-go/internal/stats"
)

// labelMapper rewrites each family's full dotted Name into the metrics_service
// tag-split form (emit_tags_as_labels=true, ADR-0264): the residual dotted name
// (tag-value segments removed, dots collapsed) + the extracted tags as
// metric[].label[] LabelPairs keyed by the Envoy DOTTED tag-name (envoy.<tag>,
// derived from stats.ExtractTags's envoy_ key via "envoy."+TrimPrefix). It reuses
// stats.ExtractTags — the SAME SN1–SN9 + SN4 matcher flattenToProm uses — so the
// metrics_service labels match the Prometheus extraction exactly. Labels apply to
// BOTH Counter and Gauge families (the knob is name-model, not value-model —
// CONTRAST the 47.2a Counter-only deltas). Stateless; one per emit_tags sink.
type labelMapper struct{}

func newLabelMapper() *labelMapper { return &labelMapper{} }

// apply returns a NEW batch with each family's Name rewritten to the dotted
// residual and metric[].label[] set to the SORTED LabelPairs. A name matching no
// SN-rule (ExtractTags error) — or one with no extracted tags — is shared by
// pointer unchanged (its full dotted name + empty labels). Like deltaState.apply it
// MUST NOT mutate the shared snapshot slice the Flusher fans to every sink: tagged
// families are rebuilt (new MetricFamily/Metric, sorted new LabelPairs), the
// Counter/Gauge value pointer shared read-only. The metric value is UNCHANGED
// (orthogonal to a preceding deltaState.apply that may have rewritten the Counter
// VALUE — this touches only Name + Label).
func (lm *labelMapper) apply(in []*dto.MetricFamily) []*dto.MetricFamily {
	out := make([]*dto.MetricFamily, 0, len(in))
	for _, fam := range in {
		residual, labels, err := stats.ExtractTags(fam.GetName())
		if err != nil || len(labels) == 0 {
			out = append(out, fam) // untagged: full name + empty labels, shared by pointer
			continue
		}
		pairs := make([]*dto.LabelPair, 0, len(labels))
		for _, l := range labels {
			pairs = append(pairs, &dto.LabelPair{
				Name:  proto.String("envoy." + strings.TrimPrefix(l.Key, "envoy_")),
				Value: proto.String(l.Value),
			})
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].GetName() < pairs[j].GetName() })
		ms := make([]*dto.Metric, 0, len(fam.GetMetric()))
		for _, m := range fam.GetMetric() {
			ms = append(ms, &dto.Metric{
				Label:       pairs,
				Counter:     m.Counter, // shared read-only (nil for gauges)
				Gauge:       m.Gauge,   // shared read-only (nil for counters)
				TimestampMs: m.TimestampMs,
			})
		}
		out = append(out, &dto.MetricFamily{Name: proto.String(residual), Type: fam.Type, Metric: ms})
	}
	return out
}
