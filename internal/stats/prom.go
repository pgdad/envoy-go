package stats

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// WriteProm walks the registry, flattens each metric via name.go's
// flattenToProm, groups by Prometheus name (status-class collapse joins the
// four _Nxx Prometheus names into one base-name group with four
// envoy_response_code_class-keyed lines per Rule SN4), sorts alphabetically by
// Prometheus name, and emits one # HELP + one # TYPE per group followed by
// one metric line per fully-qualified label set. Group separator is a blank
// line. Returns nil on success or the first io.Writer error encountered.
//
// On a flattenToProm error for any single metric (which should not happen if
// NewCounter/NewGauge validation held), the metric is silently skipped and
// the writer continues — log+ignore matches the BRAINSTORM §5.3 rationale
// that "errors from WriteProm are logged and otherwise ignored (no retry,
// no error response — too late, headers already sent)."
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

	r.Walk(func(m Metric) {
		base, labels, err := flattenToProm(m.Name())
		if err != nil {
			return // skip malformed names (defense-in-depth; should not occur)
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
