package stats

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

// FuzzPromTextFormat fuzzes the Prometheus text-format writer with adversarial
// label values. Per BRAINSTORM §6.4 + SPEC §11.8 + `## Settled SPEC §12
// deferred decisions` #11, escaping bugs are the most likely class of bug in
// the writer, and Prometheus's parser is strict. The fuzzer asserts no panic
// AND that the output round-trips through a tiny in-test Prometheus parser
// without error (no third-party Prom library — keeps the fuzzer consistent
// with ADR-0059 / D-3.2).
//
// Adversarial label values reach the writer's escape path via the synthetic-
// Metric injection technique from prom_test.go: the Registry's NewCounter /
// NewGauge paths reject adversarial names (nameRE), so the fuzzer constructs
// a synthMetric whose Name() embeds the fuzz-mutated string in the
// listener.<addr>.downstream_cx_total form — flattenToProm then extracts
// <addr> as the envoy_listener_address label value, which is the adversarial
// payload exercised by escapeLabelValue inside writeMetricLine. This mirrors
// the production data-flow (listener address bytes from a config become a
// label value at scrape time) without going through the regex-validated
// register-time path.
//
// Per ADR-0018 the budget is 30s in CI; arbitrary local time. Per ADR-0042
// fuzzers do not require their own ADR.
func FuzzPromTextFormat(f *testing.F) {
	seeds := []string{
		"plain",
		`with "quotes"`,
		`with\backslash`,
		"with\nnewline",
		"\x00null-byte",
		"trailing=equals",
		"x" + strings.Repeat("y", 4096), // very long
		"unicode-é-snowman-☃",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, labelValue string) {
		r := NewRegistry()
		// Synthetic-Metric injection — bypasses NewCounter's nameRE so the
		// adversarial label-value bytes reach flattenToProm's <addr>
		// extraction and then writeMetricLine's escapeLabelValue. The
		// internal name shape "listener.<addr>.downstream_cx_total" is
		// chosen so flattenToProm extracts <addr> as the envoy_listener_address
		// label value (Rule SN3); see prom_test.go's TestWriteProm_EscapesLabelValues
		// for the same technique applied to a fixed adversarial input.
		//
		// Dots in labelValue must be neutralized: flatten uses `.` as the
		// segment separator and embedding `.` inside the addr position would
		// split the name across the addr|rest boundary, producing a malformed
		// Prometheus name in `rest`. Replacing `.` with `_` keeps the escape
		// path on every other adversarial byte (`\`, `"`, `\n`, NUL, unicode,
		// long-string) exercised end-to-end.
		addr := strings.ReplaceAll(labelValue, ".", "_")
		name := "listener." + addr + ".downstream_cx_total"
		m := &synthMetric{
			name:   name,
			mtype:  MetricCounter,
			format: "1",
		}
		r.metrics = append(r.metrics, m)
		r.byName[name] = m

		var buf bytes.Buffer
		if err := WriteProm(&buf, r); err != nil {
			t.Fatalf("WriteProm error on label %q: %v", labelValue, err)
		}
		// Round-trip parse: every non-empty non-comment line must match
		//   <name>{<key>="<value>",...} <value>
		// or
		//   <name> <value>
		for _, line := range strings.Split(buf.String(), "\n") {
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if !validatePromLine(line) {
				t.Errorf("malformed Prom line %q from label-value seed %q", line, labelValue)
			}
		}
	})
}

// validatePromLine implements a minimal Prometheus exposition-format check.
// Returns true if line matches either:
//
//	<name> <value>
//	<name>{<key>="<escaped-value>",...} <value>
//
// where <name> matches nameRE, <value> parses as a float64, and label keys
// match nameRE. The check is generous on label-value content (the writer's
// escapeLabelValue is responsible for producing well-formed quoted strings;
// this check verifies only the outer quoting + comma separation respecting
// backslash-escapes).
//
// Splitting head from value is `}`-aware: the writer can emit unescaped
// spaces inside label values (the Prom spec only requires `\` `"` `\n` to
// be escaped), so a naive last-space split misclassifies in-quote spaces as
// the value separator. When the line contains a labels block, the value
// separator is the SPACE immediately after the matching `}`; otherwise the
// line is the simpler `<name> <value>` shape and the unique space splits.
func validatePromLine(line string) bool {
	open := strings.Index(line, "{")
	var head, value string
	if open < 0 {
		// Simple <name> <value> line.
		idx := strings.LastIndex(line, " ")
		if idx < 0 {
			return false
		}
		head = line[:idx]
		value = line[idx+1:]
	} else {
		// Labeled <name>{...} <value> line; locate the matching `}` while
		// respecting backslash-escapes inside quoted label values, then the
		// SPACE immediately after the `}` is the value separator.
		closeIdx := matchingBraceEnd(line, open)
		if closeIdx < 0 || closeIdx+1 >= len(line) || line[closeIdx+1] != ' ' {
			return false
		}
		head = line[:closeIdx+1]
		value = line[closeIdx+2:]
	}
	if _, err := strconv.ParseFloat(value, 64); err != nil {
		return false
	}
	// Head is either <name> or <name>{<labels>}.
	if !strings.Contains(head, "{") {
		return nameRE.MatchString(head)
	}
	open = strings.Index(head, "{")
	closeIdx := strings.LastIndex(head, "}")
	if closeIdx < open {
		return false
	}
	name := head[:open]
	labels := head[open+1 : closeIdx]
	if !nameRE.MatchString(name) {
		return false
	}
	// labels: comma-separated key="value" pairs (escaped).
	for _, pair := range splitLabelsRespectingEscapes(labels) {
		eq := strings.Index(pair, "=")
		if eq < 0 {
			return false
		}
		k := pair[:eq]
		v := pair[eq+1:]
		if !nameRE.MatchString(k) {
			return false
		}
		if !strings.HasPrefix(v, `"`) || !strings.HasSuffix(v, `"`) || len(v) < 2 {
			return false
		}
	}
	return true
}

// matchingBraceEnd returns the index of the `}` that closes the `{` at
// position open, respecting backslash-escapes inside double-quoted label
// values. Returns -1 if no matching close brace is found.
func matchingBraceEnd(line string, open int) int {
	inQuote := false
	prevBackslash := false
	for i := open + 1; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '\\' && inQuote && !prevBackslash:
			prevBackslash = true
			continue
		case c == '"' && !prevBackslash:
			inQuote = !inQuote
		case c == '}' && !inQuote:
			return i
		}
		prevBackslash = false
	}
	return -1
}

// splitLabelsRespectingEscapes splits a labels-block body like
//
//	a="x",b="y\"z",c="w"
//
// into ["a=\"x\"", "b=\"y\\\"z\"", "c=\"w\""], respecting backslash-escapes
// inside quoted values so that escaped commas / quotes inside a value do
// not split the pair.
func splitLabelsRespectingEscapes(s string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	prevBackslash := false
	for _, r := range s {
		switch {
		case r == '\\' && !prevBackslash:
			prevBackslash = true
			cur.WriteRune(r)
			continue
		case r == '"' && !prevBackslash:
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == ',' && !inQuote:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
		prevBackslash = false
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}
