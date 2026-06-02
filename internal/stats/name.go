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
	case strings.HasPrefix(internal, "wasm."):
		// Phase 25.1 / Task 15 follow-up: wasm filter inline-prefix detection
		// per AMEND-A2 + ADR-0202 + ADR-0203 (mirrors phase-15 bandwidth_limit's
		// inline-prefix shape — NO label promotion, stat_scope INLINED into
		// the Prometheus base name). The wasm filter's stat-name shape is:
		//
		//   wasm.<scope>.<rest>
		//
		// where <scope> is either the Group-B runtime token (`wazero` at 25.1
		// per AMEND-A1) or the Group-C envoy-go-strict per-plugin token
		// (`<plugin_name>` from the PluginConfig). Internal dots in <rest>
		// (e.g. `envoy_go.failures` segment) are converted to `_` so the
		// projected Prometheus name is valid.
		//
		// Examples:
		//
		//   wasm.wazero.created               → envoy_wasm_wazero_created
		//   wasm.wazero.active                → envoy_wasm_wazero_active
		//   wasm.plugin_e.executions          → envoy_wasm_plugin_e_executions
		//   wasm.plugin_e.hostcall_denied     → envoy_wasm_plugin_e_hostcall_denied
		//   wasm.plugin_e.envoy_go.failures   → envoy_wasm_plugin_e_envoy_go_failures
		//
		// KEEP IN SYNC with newFilterStats in internal/filter/http/wasm/stats.go
		// (the 5-counter surface per AMEND-A2 tri-group prefix structure). If a
		// future task adds new wasm.* stat names, this rule accepts them
		// uniformly without additional code changes — the rule is intentionally
		// permissive (no per-counter allow-list, unlike SN9 / bandwidth_limit).
		//
		// Per AMEND-A2 + ADR-0202 + ADR-0203.
		tail := strings.TrimPrefix(internal, "wasm.")
		if tail == "" {
			return "", nil, fmt.Errorf("stats: name %q matches wasm.* but has no <scope> segment", internal)
		}
		base = "envoy_wasm_" + strings.ReplaceAll(tail, ".", "_")
		return base, nil, nil
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
				// KEEP IN SYNC with newFilterStats / newFilterStatsIfAbsent in
				// internal/filter/http/localratelimit/local_ratelimit.go: the 4
				// counter names below MUST match the filter's emitted set.
				// Future widening (e.g., a `shadow_mode` counter when
				// filter_enforced<100% support lands) extends BOTH this switch
				// AND the filter's filterStats struct + constructors in lockstep.
				switch counter {
				case "enabled", "ok", "rate_limited", "enforced":
					labels = append(labels, Label{Key: "envoy_local_http_ratelimit_prefix", Value: prefix})
					base = "envoy_http_local_rate_limit_" + counter
					// Skip SN4 status-class collapse below (SN9 names don't have _Nxx suffix).
					return base, labels, nil
				}
			}
		}
		// Phase-15 bandwidth_limit inline-prefix detection per SPEC §11.P10 +
		// §11.P11 + §1.1 amendment 8 + ADR-0138 §Decision (iii). The internal
		// stat path `<stat_prefix>.http_bandwidth_limit.<counter>` (mirrors
		// phase-11 local_ratelimit's ADR-0118 SN9-shape) flattens to Prometheus
		// `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}` with NO label
		// extraction (stat_prefix INLINED into base name; NO tag-extractor).
		//
		// This is NOT a new SN-numbered rule per ADR-0138 §Alternative (b)
		// rejected "new SN10 rule with tag-extractor" — it is a filter-specific
		// inline-prefix detection (no label promotion; no per-segment
		// extraction; just dot→underscore substitution + envoy_ prefix). KEEP
		// IN SYNC with newFilterStats / newFilterStatsIfAbsent in
		// internal/filter/http/bandwidthlimit/bandwidthlimit.go: the 14
		// canonical names below MUST match the filter's emitted set per SPEC
		// §1.1 amendment 7 (8 counters + 6 gauges).
		//
		// Per SPEC §11.P10 + ADR-0138.
		const blSegment = ".http_bandwidth_limit."
		if idx := strings.Index(internal, blSegment); idx > 0 {
			prefix := internal[:idx]
			counter := internal[idx+len(blSegment):]
			// Validate: prefix has no dots; counter is one of the 14 known
			// active names. Per amendment 7 (8 counters + 6 gauges).
			if !strings.ContainsRune(prefix, '.') {
				switch counter {
				case "request_enabled", "request_enforced",
					"request_incoming_total_size", "request_allowed_total_size",
					"response_enabled", "response_enforced",
					"response_incoming_total_size", "response_allowed_total_size",
					"request_pending", "request_incoming_size", "request_allowed_size",
					"response_pending", "response_incoming_size", "response_allowed_size":
					// Inline-prefix shape: NO label promotion. The full
					// dot→underscore substitution produces the Prometheus base.
					base = "envoy_" + strings.ReplaceAll(internal, ".", "_")
					// Skip SN4 status-class collapse (bandwidth_limit names
					// do not carry _Nxx suffixes).
					return base, nil, nil
				}
			}
		}
		// Network rbac_network filter tag-extractor (phase 26.3 / ADR-0218). The
		// envoy.filters.network.rbac filter roots its four counters on the
		// PGV-required `stat_prefix` (NOT an HCM — the HTTP rbac filter's stats
		// flow through the SN2 `http.<hcm>.rbac.*` path and never reach this
		// default branch). The internal stat path is:
		//
		//   <stat_prefix>.rbac.{allowed,denied}
		//   <stat_prefix>.rbac.[<shadow_rules_stat_prefix>.]{shadow_allowed,shadow_denied}
		//
		// where <stat_prefix> is a single segment (no dots). This mirrors
		// reference Envoy v1.37.2's tag-extractor for the network rbac filter
		// (empirically captured at phase-26.3 Task 14 via the dockerized
		// differential): the stat_prefix is promoted to a LABEL
		// (`envoy_rbac_prefix=<stat_prefix>`) and the remainder
		// (`rbac.[<shadow_prefix>.]<counter>`) flattens to the base name
		// `envoy_rbac_<rest>` (dots→underscores). Examples (ref-parity):
		//
		//   rbac_allow.rbac.allowed                  → envoy_rbac_allowed{envoy_rbac_prefix="rbac_allow"}
		//   rbac_deny.rbac.denied                    → envoy_rbac_denied{envoy_rbac_prefix="rbac_deny"}
		//   rbac_shadow.rbac.shadow_ns.shadow_denied → envoy_rbac_shadow_ns_shadow_denied{envoy_rbac_prefix="rbac_shadow"}
		//
		// KEEP IN SYNC with newFilterStats in
		// internal/filter/network/rbac/rbac.go (the four-counter surface). Adding
		// a 5th counter requires editing BOTH this suffix allowlist (the
		// strings.HasSuffix chain below) AND the filterStats struct +
		// newFilterStats registration in rbac.go in lockstep. The detection is a
		// second-pass on the unmatched-prefix path (after the SN1-SN5 switch
		// fails); the SN1-SN5 hot-path is unchanged.
		const rbacSegment = ".rbac."
		if idx := strings.Index(internal, rbacSegment); idx > 0 {
			prefix := internal[:idx]
			rest := internal[idx+1:] // "rbac.<...>" — keep the literal rbac. segment in the base
			// Validate: prefix has no dots (single stat_prefix segment); rest
			// ends in one of the four known network-rbac counter names (the
			// optional shadow_rules_stat_prefix segment sits between `rbac.` and
			// the shadow_* counter — accepted by the suffix check).
			if !strings.ContainsRune(prefix, '.') &&
				(strings.HasSuffix(rest, ".allowed") || strings.HasSuffix(rest, ".denied") ||
					strings.HasSuffix(rest, ".shadow_allowed") || strings.HasSuffix(rest, ".shadow_denied")) {
				labels = append(labels, Label{Key: "envoy_rbac_prefix", Value: prefix})
				base = "envoy_" + strings.ReplaceAll(rest, ".", "_")
				// Skip SN4 status-class collapse (rbac names have no _Nxx suffix).
				return base, labels, nil
			}
		}
		// Phase-28.1 zookeeper_proxy INLINE-PREFIX detection (ADR-0222; parent AMEND-A4;
		// the ADR-0138 bandwidth_limit + wasm.* permissive-shape precedents). Internal
		// name <stat_prefix>.zookeeper.<counter> (counter MAY contain dots — the dynamic
		// auth.<scheme>_rq family) flattens to envoy_<stat_prefix>_zookeeper_<counter>
		// with NO label promotion (upstream applies no tag extraction to this filter;
		// its Prometheus exposition is flat with an empty label set).
		// Validation is the SHAPE (D-P8): a .zookeeper. segment with a dot-free head —
		// no per-counter allowlist (201 static names + an open-ended dynamic family
		// make an allowlist unmaintainable; the wasm. arm at :88-122 is the
		// established permissive precedent). Documented acceptance: any future stat
		// named <x>.zookeeper.<y> from another subsystem matches this arm.
		// KEEP-IN-SYNC: internal/filter/network/zookeeperproxy/stats.go (the roster).
		const zkSegment = ".zookeeper."
		if idx := strings.Index(internal, zkSegment); idx > 0 {
			prefix := internal[:idx]
			if !strings.ContainsRune(prefix, '.') {
				base = "envoy_" + strings.ReplaceAll(internal, ".", "_")
				return base, nil, nil
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
