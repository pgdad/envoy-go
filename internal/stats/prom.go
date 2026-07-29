package stats

import (
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
)

// promSkipLogFmt and promSkipLogSep pin the aggregate skip-report line emitted
// by WriteProm. They are named constants, not inline literals, so the format
// AND the separator are both pinnable: a byte figure quoted for this line is
// unfalsifiable without the separator, since the join contributes
// len(sep)*(n-1) bytes on its own.
const (
	promSkipLogFmt = "stats: WriteProm skipped %d registered metric name(s) with no recognized top-level segment: %s"
	promSkipLogSep = ", "
)

// WriteProm walks the registry, flattens each metric via name.go's
// flattenToProm, groups by Prometheus name (status-class collapse joins the
// four _Nxx Prometheus names into one base-name group with four
// envoy_response_code_class-keyed lines per Rule SN4), sorts alphabetically by
// Prometheus name, and emits one # HELP + one # TYPE per group followed by
// one metric line per fully-qualified label set. Group separator is a blank
// line. Returns nil on success or the first io.Writer error encountered.
//
// On a flattenToProm error for any single metric, that metric is omitted from
// the exposition and the writer continues — no retry and no error response,
// since the headers are already sent by the time WriteProm runs. The skipped
// names are NOT lost: they are collected during the walk and reported in
// AGGREGATE, as exactly ONE log line per WriteProm call, emitted after the walk
// completes and only when the set is non-empty. Sorted, so the line is stable
// across runs.
//
// The aggregate shape is deliberate. A per-metric log would emit one line per
// skipped name per scrape, and a skip COUNTER cannot be used at all: Registry
// .Walk holds r.mu.RLock across every callback while getOrRegister takes
// r.mu.Lock, and Go's RWMutex is not reentrant, so registering a stat from
// inside the walk DEADLOCKS the scrape it instruments.
//
// The stat surface is unchanged by this reporting path: +0.
func WriteProm(w io.Writer, r *Registry) error {
	type promLine struct {
		labels []Label
		value  string
	}
	type promGroup struct {
		name    string
		mtype   MetricType
		help    string
		entries []promLine
	}
	groups := make(map[string]*promGroup)
	var keys []string
	var skipped []string

	r.Walk(func(m Metric) {
		base, labels, err := flattenToProm(m.Name())
		if err != nil {
			// Collect, do NOT log here: this callback runs under the
			// Registry read lock, and the aggregate line is emitted once
			// after the walk returns.
			skipped = append(skipped, m.Name())
			return
		}
		g, ok := groups[base]
		if !ok {
			g = &promGroup{
				name:  base,
				mtype: m.Type(),
				help:  helpText[base], // empty string if absent; the emit loop falls back to the Prometheus name
			}
			groups[base] = g
			keys = append(keys, base)
		}
		g.entries = append(g.entries, promLine{labels: labels, value: m.Format()})
	})
	if len(skipped) > 0 {
		sort.Strings(skipped)
		log.Printf(promSkipLogFmt, len(skipped), strings.Join(skipped, promSkipLogSep))
	}
	sort.Strings(keys)

	for i, k := range keys {
		g := groups[k]
		help := g.help
		if help == "" {
			help = g.name // fall back to the name as a no-op help when missing
		}
		typeStr := "counter"
		if g.mtype == MetricGauge {
			typeStr = "gauge"
		}
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", g.name, help, g.name, typeStr); err != nil {
			return err
		}
		for _, e := range g.entries {
			if err := writeMetricLine(w, g.name, e.labels, e.value); err != nil {
				return err
			}
		}
		if i < len(keys)-1 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeMetricLine emits one Prometheus metric line:
//
//	<name>{<key>="<escaped_value>",...} <value>\n
//
// When labels is empty, the {} block is OMITTED:
//
//	<name> <value>\n
func writeMetricLine(w io.Writer, name string, labels []Label, value string) error {
	if len(labels) == 0 {
		_, err := fmt.Fprintf(w, "%s %s\n", name, value)
		return err
	}
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteByte('{')
	for i, l := range labels {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(l.Key)
		sb.WriteString(`="`)
		sb.WriteString(escapeLabelValue(l.Value))
		sb.WriteByte('"')
	}
	sb.WriteByte('}')
	sb.WriteByte(' ')
	sb.WriteString(value)
	sb.WriteByte('\n')
	_, err := io.WriteString(w, sb.String())
	return err
}
