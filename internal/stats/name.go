package stats

import (
	"fmt"
	"regexp"
	"strings"
)

// Label is one Prometheus label key/value pair. Label-set ordering inside a
// single metric line is determined by the writer (prom.go) — the contract is
// stable within a Prometheus name group; the order is not asserted by tests
// at the per-label level beyond set-equality.
type Label struct {
	Key   string
	Value string
}

// statusClassRE matches the trailing _Nxx (N ∈ 1..5) per Rule SN4. Capture
// group 1 is the base (without the _Nxx); capture group 2 is the single
// class digit. The regex is anchored at end of string.
var statusClassRE = regexp.MustCompile(`^(.+)_([1-5])xx$`)

// flattenToProm transforms an internal hierarchical-dotted name to the
// Prometheus exposition form per Rules SN1–SN8 (ADR-0061; SPEC §10.1).
//
//	SN1: cluster.<n>.<rest>     → envoy_cluster_<rest> + label envoy_cluster_name=<n>
//	SN2: http.<stat_prefix>.<rest> → envoy_http_<rest> + label envoy_http_conn_manager_prefix=<stat_prefix>
//	SN3: listener.<addr>.<rest> → envoy_listener_<rest> + label envoy_listener_address=<addr>
//	SN4: <base>_Nxx             → <base>_xx + label envoy_response_code_class=N (N ∈ 1..5)
//	SN5: server.<rest>          → envoy_server_<rest> + no labels
//	SN6: HELP text best-effort English (handled by prom.go via helpText map)
//	SN7: histograms not emitted (Task-2-time NewCounter/NewGauge are the only
//	     registry methods; absence is the contract)
//	SN8: per-endpoint cluster stats not emitted (similarly absent)
//
// Returns the Prometheus base name + the extracted label set + nil on success.
// Returns "", nil, error on names that match no top-level rule.
func flattenToProm(internal string) (string, []Label, error) {
	var labels []Label
	var rest, base string
	switch {
	case strings.HasPrefix(internal, "cluster."):
		// Rule SN1
		tail := strings.TrimPrefix(internal, "cluster.")
		dot := strings.Index(tail, ".")
		if dot < 0 {
			return "", nil, fmt.Errorf("stats: name %q matches cluster.* but has no <rest> segment", internal)
		}
		labels = append(labels, Label{Key: "envoy_cluster_name", Value: tail[:dot]})
		rest = tail[dot+1:]
		base = "envoy_cluster_" + rest
	case strings.HasPrefix(internal, "http."):
		// Rule SN2
		tail := strings.TrimPrefix(internal, "http.")
		dot := strings.Index(tail, ".")
		if dot < 0 {
			return "", nil, fmt.Errorf("stats: name %q matches http.* but has no <rest> segment", internal)
		}
		labels = append(labels, Label{Key: "envoy_http_conn_manager_prefix", Value: tail[:dot]})
		// Phase 09 / Task 14 follow-up: SN2 internal-dot transform — Prometheus
		// metric names cannot contain `.`, so any remaining `.` in the rest
		// segment of HCM-scoped stats is converted to `_` before forming the
		// base. ADR-0061's SN1–SN8 list is unchanged in spirit; this is an
		// SN2 implementation detail surfaced by phase 09's
		// `http.<sp>.fault.<metric>` keying — the first HCM-scoped stat with
		// a nested rest segment containing internal dots. (SN1 is the
		// `cluster.*` rule; SN3 is the `listener.*` rule; SN5 is the
		// `server.*` rule; the dot→underscore transform applies wherever a
		// nested rest is introduced — currently SN2 only.) Confirmed by
		// SPEC §11.6 empirical pin: reference Envoy v1.37.2 emits
		// `envoy_http_fault_aborts_injected{...}` (underscore, not period).
		rest = strings.ReplaceAll(tail[dot+1:], ".", "_")
		base = "envoy_http_" + rest
	case strings.HasPrefix(internal, "listener."):
		// Rule SN3
		tail := strings.TrimPrefix(internal, "listener.")
		dot := strings.Index(tail, ".")
		if dot < 0 {
			return "", nil, fmt.Errorf("stats: name %q matches listener.* but has no <rest> segment", internal)
		}
		labels = append(labels, Label{Key: "envoy_listener_address", Value: tail[:dot]})
		rest = tail[dot+1:]
		base = "envoy_listener_" + rest
	case strings.HasPrefix(internal, "server."):
		// Rule SN5
		rest = strings.TrimPrefix(internal, "server.")
		base = "envoy_server_" + rest
	default:
		// Rule SN9 (added per phase 11 ADR-0118 + ADR-0061 amendment): the
		// local_ratelimit filter-specific tag-extractor matches names of the
		// shape `<stat_prefix>.http_local_rate_limit.<counter>` where
		// <stat_prefix> is a single segment (no dots) and <counter> is one of
		// {enabled, ok, rate_limited, enforced}. Produces Prometheus base name
		// `envoy_http_local_rate_limit_<counter>` + label
		// `envoy_local_http_ratelimit_prefix=<stat_prefix>`.
		//
		// The rule is a SECOND-PASS detection — fires only on the unmatched-
		// prefix path (after SN1-SN5 prefix-segment switch fails). The existing
		// SN1-SN5 hot-path is unchanged.
		//
		// Per SPEC §11.5 + ADR-0118.
		const lrlSegment = ".http_local_rate_limit."
		if idx := strings.Index(internal, lrlSegment); idx > 0 {
			prefix := internal[:idx]
			counter := internal[idx+len(lrlSegment):]
			// Validate: prefix has no dots; counter is one of the 4 known names.
			if !strings.ContainsRune(prefix, '.') {
				switch counter {
				case "enabled", "ok", "rate_limited", "enforced":
					labels = append(labels, Label{Key: "envoy_local_http_ratelimit_prefix", Value: prefix})
					base = "envoy_http_local_rate_limit_" + counter
					// Skip SN4 status-class collapse below (SN9 names don't have _Nxx suffix).
					return base, labels, nil
				}
			}
		}
		return "", nil, fmt.Errorf("stats: name %q has no recognized top-level segment (want cluster.|http.|listener.|server.)", internal)
	}

	// Rule SN4: detect the trailing _Nxx and split.
	if m := statusClassRE.FindStringSubmatch(base); m != nil {
		base = m[1] + "_xx"
		labels = append([]Label{{Key: "envoy_response_code_class", Value: m[2]}}, labels...)
	}

	return base, labels, nil
}

// escapeLabelValue escapes a label value per the Prometheus text-format spec:
//
//	\  → \\
//	"  → \"
//	\n → \n  (literal two-char backslash-n in the output)
//
// Other characters pass through unchanged.
func escapeLabelValue(s string) string {
	if !strings.ContainsAny(s, `\"`+"\n") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// helpText maps each Prometheus name emitted by 06.1 to a static English
// description per BRAINSTORM §4.5. Per Rule SN6, HELP text is NOT byte-equal
// to Envoy's HELP text — the differential equivalence claim is on values +
// label keys + types only. The 11 entries cover the 13 unique Prometheus
// names emitted by 06.1 (the four _Nxx counters per HCM and per cluster
// collapse to envoy_http_downstream_rq_xx and envoy_cluster_upstream_rq_xx
// respectively per Rule SN4) plus one 06.2 backpressure counter.
var helpText = map[string]string{
	"envoy_listener_downstream_cx_total":  "Total connections accepted on the listener.",
	"envoy_listener_downstream_cx_active": "Active connections on the listener.",
	"envoy_http_downstream_rq_total":      "Total requests received by the HTTP connection manager.",
	"envoy_http_downstream_rq_xx":         "Requests received by the HTTP connection manager, by response code class.",
	"envoy_cluster_upstream_rq_total":     "Total requests dispatched to upstream clusters.",
	"envoy_cluster_upstream_rq_xx":        "Requests dispatched to upstream clusters, by response code class.",
	"envoy_cluster_upstream_cx_total":     "Total connections established to upstream clusters.",
	"envoy_cluster_upstream_cx_active":    "Active connections to upstream clusters.",
	"envoy_cluster_membership_total":      "Number of endpoints in the cluster.",
	"envoy_server_live":                   "1 if the server is live, 0 otherwise.",
	"envoy_server_accesslog_dropped":      "Total access-log records dropped due to backpressure (per-process aggregate across all sinks).",
}
