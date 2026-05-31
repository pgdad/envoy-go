package stats

import (
	"reflect"
	"strings"
	"testing"
)

func TestFlattenToProm_Listener(t *testing.T) {
	prom, labels, err := flattenToProm("listener.0_0_0_0_10000.downstream_cx_total")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if prom != "envoy_listener_downstream_cx_total" {
		t.Errorf("promName = %q, want envoy_listener_downstream_cx_total", prom)
	}
	want := []Label{{Key: "envoy_listener_address", Value: "0_0_0_0_10000"}}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("labels = %+v, want %+v", labels, want)
	}
}

func TestFlattenToProm_HCM(t *testing.T) {
	prom, labels, err := flattenToProm("http.ingress_http.downstream_rq_total")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if prom != "envoy_http_downstream_rq_total" {
		t.Errorf("promName = %q, want envoy_http_downstream_rq_total", prom)
	}
	want := []Label{{Key: "envoy_http_conn_manager_prefix", Value: "ingress_http"}}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("labels = %+v, want %+v", labels, want)
	}
}

// TestFlattenToProm_HCM_DottedRest exercises the SN2 internal-dot transform
// (Phase 09 / Task 14 follow-up). The HCM stat `http.<sp>.fault.<metric>` has
// a nested rest segment (`fault.aborts_injected`); its internal `.` must be
// converted to `_` so the projected Prometheus metric name is valid (Prom
// names cannot contain `.`). The `<sp>` is extracted as the
// `envoy_http_conn_manager_prefix` label, NOT included in the metric name.
func TestFlattenToProm_HCM_DottedRest(t *testing.T) {
	prom, labels, err := flattenToProm("http.ingress_http.fault.aborts_injected")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if prom != "envoy_http_fault_aborts_injected" {
		t.Errorf("promName = %q, want %q", prom, "envoy_http_fault_aborts_injected")
	}
	want := []Label{{Key: "envoy_http_conn_manager_prefix", Value: "ingress_http"}}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("labels = %+v, want %+v", labels, want)
	}
}

func TestFlattenToProm_Cluster(t *testing.T) {
	prom, labels, err := flattenToProm("cluster.c0.upstream_cx_active")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if prom != "envoy_cluster_upstream_cx_active" {
		t.Errorf("promName = %q, want envoy_cluster_upstream_cx_active", prom)
	}
	want := []Label{{Key: "envoy_cluster_name", Value: "c0"}}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("labels = %+v, want %+v", labels, want)
	}
}

// Rule SN4 — the empirical-verification gate per SPEC §10.1. The trailing
// class digit is STRIPPED from the metric name (so "_2xx" → base ending
// "_xx"); label name is "envoy_response_code_class"; label value is the
// single class digit as a string.
func TestFlattenToProm_StatusClass_HCM(t *testing.T) {
	prom, labels, err := flattenToProm("http.ingress_http.downstream_rq_2xx")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if prom != "envoy_http_downstream_rq_xx" {
		t.Errorf("promName = %q, want envoy_http_downstream_rq_xx (Rule SN4: digit stripped, base ends _xx)", prom)
	}
	wantSet := map[string]string{
		"envoy_response_code_class":      "2",
		"envoy_http_conn_manager_prefix": "ingress_http",
	}
	gotSet := make(map[string]string)
	for _, l := range labels {
		gotSet[l.Key] = l.Value
	}
	if !reflect.DeepEqual(wantSet, gotSet) {
		t.Errorf("labels = %+v, want %+v", gotSet, wantSet)
	}
}

func TestFlattenToProm_StatusClass_Cluster_AllDigits(t *testing.T) {
	for digit := 1; digit <= 5; digit++ {
		internal := "cluster.c0.upstream_rq_" + string(rune('0'+digit)) + "xx"
		t.Run(internal, func(t *testing.T) {
			prom, labels, err := flattenToProm(internal)
			if err != nil {
				t.Fatalf("flattenToProm(%q): %v", internal, err)
			}
			if prom != "envoy_cluster_upstream_rq_xx" {
				t.Errorf("promName = %q, want envoy_cluster_upstream_rq_xx", prom)
			}
			wantClass := string(rune('0' + digit))
			var gotClass, gotName string
			for _, l := range labels {
				switch l.Key {
				case "envoy_response_code_class":
					gotClass = l.Value
				case "envoy_cluster_name":
					gotName = l.Value
				}
			}
			if gotClass != wantClass {
				t.Errorf("envoy_response_code_class = %q, want %q", gotClass, wantClass)
			}
			if gotName != "c0" {
				t.Errorf("envoy_cluster_name = %q, want c0", gotName)
			}
		})
	}
}

func TestFlattenToProm_Server(t *testing.T) {
	prom, labels, err := flattenToProm("server.live")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if prom != "envoy_server_live" {
		t.Errorf("promName = %q, want envoy_server_live", prom)
	}
	if len(labels) != 0 {
		t.Errorf("labels = %+v, want empty (Rule SN5: server.<rest> has no extracted labels)", labels)
	}
}

func TestFlattenToProm_Invalid_NoMatchingRule(t *testing.T) {
	_, _, err := flattenToProm("unknown_top_segment.foo")
	if err == nil {
		t.Error("expected error for unknown top segment; got nil")
	}
}

func TestEscapeLabelValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{`with "quotes"`, `with \"quotes\"`},
		{`with\backslash`, `with\\backslash`},
		{"with\nnewline", `with\nnewline`},
		{`all "\` + "\n" + `together`, `all \"\\\ntogether`},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := escapeLabelValue(tc.in)
			if got != tc.want {
				t.Errorf("escapeLabelValue(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestHelpText_Coverage(t *testing.T) {
	wantNames := []string{
		"envoy_listener_downstream_cx_total",
		"envoy_listener_downstream_cx_active",
		"envoy_http_downstream_rq_total",
		"envoy_http_downstream_rq_xx",
		"envoy_cluster_upstream_rq_total",
		"envoy_cluster_upstream_rq_xx",
		"envoy_cluster_upstream_cx_total",
		"envoy_cluster_upstream_cx_active",
		"envoy_cluster_membership_total",
		"envoy_server_live",
	}
	for _, n := range wantNames {
		if _, ok := helpText[n]; !ok {
			t.Errorf("helpText missing entry for %q", n)
		}
	}
}

func TestHelpText_AccessLogDropped(t *testing.T) {
	want := "Total access-log records dropped due to backpressure (per-process aggregate across all sinks)."
	if got := helpText["envoy_server_accesslog_dropped"]; got != want {
		t.Errorf("helpText[envoy_server_accesslog_dropped] = %q, want %q", got, want)
	}
}

// SN9 tests (phase 11 / ADR-0118): local_ratelimit filter-specific tag-extractor.

func TestFlattenToProm_SN9_BasicStatPrefix(t *testing.T) {
	base, labels, err := flattenToProm("foo.http_local_rate_limit.enabled")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if base != "envoy_http_local_rate_limit_enabled" {
		t.Errorf("base: got %q, want %q", base, "envoy_http_local_rate_limit_enabled")
	}
	if len(labels) != 1 || labels[0].Key != "envoy_local_http_ratelimit_prefix" || labels[0].Value != "foo" {
		t.Errorf("labels: got %v, want [envoy_local_http_ratelimit_prefix=foo]", labels)
	}
}

func TestFlattenToProm_SN9_AllFourCounters(t *testing.T) {
	for _, counter := range []string{"enabled", "ok", "rate_limited", "enforced"} {
		t.Run(counter, func(t *testing.T) {
			base, labels, err := flattenToProm("test.http_local_rate_limit." + counter)
			if err != nil {
				t.Fatalf("flattenToProm: %v", err)
			}
			wantBase := "envoy_http_local_rate_limit_" + counter
			if base != wantBase {
				t.Errorf("base: got %q, want %q", base, wantBase)
			}
			if len(labels) != 1 || labels[0].Value != "test" {
				t.Errorf("labels: got %v, want envoy_local_http_ratelimit_prefix=test", labels)
			}
		})
	}
}

func TestFlattenToProm_SN9_PrefixWithUnderscores(t *testing.T) {
	base, labels, err := flattenToProm("my_prefix.http_local_rate_limit.ok")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if base != "envoy_http_local_rate_limit_ok" {
		t.Errorf("base: got %q, want %q", base, "envoy_http_local_rate_limit_ok")
	}
	if len(labels) != 1 || labels[0].Value != "my_prefix" {
		t.Errorf("labels: got %v, want value my_prefix", labels)
	}
}

func TestFlattenToProm_SN9_DoesNotConflictWithSN1234(t *testing.T) {
	// SN1 (cluster.) wins over SN9 even if name contains the SN9 segment.
	base, labels, err := flattenToProm("cluster.foo.http_local_rate_limit.enabled")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	// SN1 produces envoy_cluster_<rest>; load-bearing claim: SN1 wins, NOT SN9.
	if !strings.HasPrefix(base, "envoy_cluster_") {
		t.Errorf("base: got %q, want SN1-prefixed envoy_cluster_*", base)
	}
	if len(labels) == 0 || labels[0].Key != "envoy_cluster_name" {
		t.Errorf("labels: got %v, want envoy_cluster_name=foo (SN1 wins)", labels)
	}
}

func TestFlattenToProm_SN9_RejectsUnknownCounter(t *testing.T) {
	// SN9 only matches the 4 known counter names; other suffixes fall through to error.
	_, _, err := flattenToProm("foo.http_local_rate_limit.unknown")
	if err == nil {
		t.Error("flattenToProm with unknown counter: want error, got nil")
	}
}

func TestFlattenToProm_SN9_RejectsLeadingDot(t *testing.T) {
	// Leading-dot input: idx == 0 (segment match starts at position 0); the
	// `idx > 0` guard rejects this so prefix can never be empty. The name
	// falls through to the error return.
	_, _, err := flattenToProm(".http_local_rate_limit.enabled")
	if err == nil {
		t.Error("flattenToProm with leading-dot prefix: want error, got nil (idx==0 must reject)")
	}
}

func TestFlattenToProm_SN9_RejectsDoublyNestedSegment(t *testing.T) {
	// Degenerate input: stat_prefix that itself contains the SN9 segment
	// substring. strings.Index returns the FIRST occurrence, so prefix becomes
	// "a" and counter becomes "b.http_local_rate_limit.enabled". The counter
	// switch rejects "b.http_local_rate_limit.enabled" (not in the 4-name
	// set), so the name falls through to the error return.
	_, _, err := flattenToProm("a.http_local_rate_limit.b.http_local_rate_limit.enabled")
	if err == nil {
		t.Error("flattenToProm with doubly-nested SN9 segment: want error, got nil")
	}
}

// Phase-15 bandwidth_limit inline-prefix detection tests (ADR-0138 + SPEC
// §11.P10 + §11.P11). Mirrors phase-11's SN9 test discipline above; the
// bandwidth_limit shape `<stat_prefix>.http_bandwidth_limit.<counter>` is a
// structural parallel to SN9 BUT differs in TWO load-bearing ways:
//
//	(a) NO label promotion — stat_prefix is INLINED into the Prometheus base
//	    name (`envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}` with NO
//	    labels), NOT extracted as a tag-extractor-driven label.
//	(b) 14 canonical counter+gauge names (8 counters + 6 gauges per amendment
//	    7), not the 4 SN9 names.
//
// Per ADR-0138 §Alternative (b) rejected "new SN10 rule with tag-extractor"
// — this is a filter-specific inline-prefix detection, NOT a new SN-numbered
// rule. The tests below pin the boundary behavior at the flattenToProm layer
// (the higher-level WriteProm pipeline tests in bandwidthlimit_test.go
// exercise the same code via a different entry point).

func TestFlattenToProm_BandwidthLimit_Basic(t *testing.T) {
	base, labels, err := flattenToProm("default.http_bandwidth_limit.request_enabled")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	want := "envoy_default_http_bandwidth_limit_request_enabled"
	if base != want {
		t.Errorf("base: got %q, want %q", base, want)
	}
	if len(labels) != 0 {
		t.Errorf("labels: got %v, want [] (inline-prefix: stat_prefix is INLINED into base, NOT a label)", labels)
	}
}

func TestFlattenToProm_BandwidthLimit_AllFourteenSuffixes(t *testing.T) {
	// Per ADR-0138 §Decision (i) + SPEC §1.1 amendment 7: 8 counters + 6
	// gauges. KEEP IN SYNC with name.go's blSegment switch + bandwidthlimit.go's
	// newFilterStats / newFilterStatsIfAbsent.
	counters := []string{
		// 8 counters.
		"request_enabled", "request_enforced",
		"request_incoming_total_size", "request_allowed_total_size",
		"response_enabled", "response_enforced",
		"response_incoming_total_size", "response_allowed_total_size",
		// 6 gauges.
		"request_pending", "request_incoming_size", "request_allowed_size",
		"response_pending", "response_incoming_size", "response_allowed_size",
	}
	if len(counters) != 14 {
		t.Fatalf("test setup: want exactly 14 counter+gauge names, got %d", len(counters))
	}
	for _, c := range counters {
		t.Run(c, func(t *testing.T) {
			internal := "test_prefix.http_bandwidth_limit." + c
			base, labels, err := flattenToProm(internal)
			if err != nil {
				t.Fatalf("flattenToProm(%q): %v", internal, err)
			}
			wantBase := "envoy_test_prefix_http_bandwidth_limit_" + c
			if base != wantBase {
				t.Errorf("base: got %q, want %q", base, wantBase)
			}
			if len(labels) != 0 {
				t.Errorf("labels: got %v, want [] (no label promotion)", labels)
			}
		})
	}
}

func TestFlattenToProm_BandwidthLimit_PrefixWithUnderscores(t *testing.T) {
	// stat_prefix can contain underscores (only `.` is the segment separator).
	// The dot→underscore substitution applies to the entire internal name;
	// existing underscores in stat_prefix pass through unchanged.
	base, labels, err := flattenToProm("route_override.http_bandwidth_limit.response_pending")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	want := "envoy_route_override_http_bandwidth_limit_response_pending"
	if base != want {
		t.Errorf("base: got %q, want %q", base, want)
	}
	if len(labels) != 0 {
		t.Errorf("labels: got %v, want []", labels)
	}
}

func TestFlattenToProm_BandwidthLimit_RejectsUnknownCounter(t *testing.T) {
	// The blSegment switch only matches the 14 canonical names; any other
	// suffix falls through to the default error return.
	_, _, err := flattenToProm("default.http_bandwidth_limit.bogus_counter")
	if err == nil {
		t.Error("flattenToProm with unknown counter: want error, got nil")
	}
}

func TestFlattenToProm_BandwidthLimit_RejectsLeadingDot(t *testing.T) {
	// Leading-dot input: idx == 0 (segment match starts at position 0); the
	// `idx > 0` guard rejects this so prefix can never be empty. The name
	// falls through to the error return.
	_, _, err := flattenToProm(".http_bandwidth_limit.request_enabled")
	if err == nil {
		t.Error("flattenToProm with leading-dot prefix: want error, got nil (idx==0 must reject)")
	}
}

func TestFlattenToProm_BandwidthLimit_RejectsDoublyNestedSegment(t *testing.T) {
	// Degenerate input: stat_prefix that itself contains a `.` (multi-segment
	// prefix). The `!strings.ContainsRune(prefix, '.')` guard rejects this;
	// the name falls through to the error return.
	_, _, err := flattenToProm("default.http_bandwidth_limit.foo.bar.request_enabled")
	if err == nil {
		t.Error("flattenToProm with multi-segment prefix: want error, got nil")
	}
}

// Phase 25.1 / Task 15 follow-up: wasm.* inline-prefix detection tests per
// AMEND-A2 + ADR-0202 + ADR-0203. Mirrors phase-15's bandwidth_limit inline-
// prefix discipline (NO label promotion; <scope> INLINED into base; internal
// dots in <rest> converted to underscores).
//
// Without this rule the wasm filter's 5-counter surface (registered as
// `wasm.<scope>.<rest>` per stats.go) would fail flattenToProm and be
// silently dropped from the /stats/prometheus exposition (per prom.go
// WriteProm's err-skip discipline). The fixture-0034 scenario (e) cross-side
// stats assertion REQUIRES the subject side to expose
// `envoy_wasm_plugin_e_executions` after one probe — the projection below
// is the load-bearing path for that assertion.

func TestFlattenToProm_Wasm_GroupB_Created(t *testing.T) {
	base, labels, err := flattenToProm("wasm.wazero.created")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if base != "envoy_wasm_wazero_created" {
		t.Errorf("base: got %q, want %q", base, "envoy_wasm_wazero_created")
	}
	if len(labels) != 0 {
		t.Errorf("labels: got %v, want [] (inline-prefix: <scope> INLINED, no label promotion)", labels)
	}
}

func TestFlattenToProm_Wasm_GroupB_Active(t *testing.T) {
	base, labels, err := flattenToProm("wasm.wazero.active")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if base != "envoy_wasm_wazero_active" {
		t.Errorf("base: got %q, want %q", base, "envoy_wasm_wazero_active")
	}
	if len(labels) != 0 {
		t.Errorf("labels: got %v, want []", labels)
	}
}

func TestFlattenToProm_Wasm_GroupC_PerPluginExecutions(t *testing.T) {
	base, labels, err := flattenToProm("wasm.plugin_e.executions")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if base != "envoy_wasm_plugin_e_executions" {
		t.Errorf("base: got %q, want %q", base, "envoy_wasm_plugin_e_executions")
	}
	if len(labels) != 0 {
		t.Errorf("labels: got %v, want []", labels)
	}
}

func TestFlattenToProm_Wasm_GroupC_PerPluginHostcallDenied(t *testing.T) {
	base, labels, err := flattenToProm("wasm.plugin_a.hostcall_denied")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if base != "envoy_wasm_plugin_a_hostcall_denied" {
		t.Errorf("base: got %q, want %q", base, "envoy_wasm_plugin_a_hostcall_denied")
	}
	if len(labels) != 0 {
		t.Errorf("labels: got %v, want []", labels)
	}
}

// TestFlattenToProm_Wasm_GroupC_InternalDotInRest covers the
// `wasm.<plugin>.envoy_go.failures` counter — its <rest> segment carries
// an internal `.` (`envoy_go.failures`) marking the envoy-go-strict origin
// of the metric per AMEND-A2. The internal dot MUST be converted to `_` so
// the projected Prometheus name is valid.
func TestFlattenToProm_Wasm_GroupC_InternalDotInRest(t *testing.T) {
	base, labels, err := flattenToProm("wasm.plugin_e.envoy_go.failures")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if base != "envoy_wasm_plugin_e_envoy_go_failures" {
		t.Errorf("base: got %q, want %q", base, "envoy_wasm_plugin_e_envoy_go_failures")
	}
	if len(labels) != 0 {
		t.Errorf("labels: got %v, want []", labels)
	}
}

func TestFlattenToProm_Wasm_RejectsEmptyScope(t *testing.T) {
	// `wasm.` alone with no scope segment falls through to the explicit
	// no-scope error.
	_, _, err := flattenToProm("wasm.")
	if err == nil {
		t.Error("flattenToProm(\"wasm.\"): want error, got nil")
	}
}

// Phase 26.3 network-rbac tag-extractor tests (ADR-0218 + SPEC §7 + §7.1).
// The network rbac_network filter roots its four counters on a single-segment
// stat_prefix: `<stat_prefix>.rbac.{allowed,denied}` and
// `<stat_prefix>.rbac.[<shadow_prefix>.]shadow_{allowed,denied}`.
// The stat_prefix is promoted to `envoy_rbac_prefix=<stat_prefix>`; the rest
// flattens to `envoy_rbac_<rest>` (dots→underscores).
//
// These tests mirror the SN9 guard discipline: each test would FAIL if the
// corresponding guard (idx>0, !ContainsRune(prefix,'.'), suffix allowlist)
// were removed.

func TestFlattenToProm_NetworkRBAC_AllFourCounters(t *testing.T) {
	// POSITIVE: all four base counter names match and produce the expected base +
	// label. Per SPEC §7 / ADR-0218: enforced counters use the plain
	// `<sp>.rbac.<counter>` shape; shadow counters are also confirmed here
	// (without a shadow_rules_stat_prefix middle segment, i.e. empty prefix path).
	cases := []struct {
		input    string
		wantBase string
		wantPfx  string
	}{
		{"rbac_allow.rbac.allowed", "envoy_rbac_allowed", "rbac_allow"},
		{"rbac_deny.rbac.denied", "envoy_rbac_denied", "rbac_deny"},
		{"rbac_shadow.rbac.shadow_allowed", "envoy_rbac_shadow_allowed", "rbac_shadow"},
		{"rbac_shadow.rbac.shadow_denied", "envoy_rbac_shadow_denied", "rbac_shadow"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			base, labels, err := flattenToProm(tc.input)
			if err != nil {
				t.Fatalf("flattenToProm(%q): %v", tc.input, err)
			}
			if base != tc.wantBase {
				t.Errorf("base: got %q, want %q", base, tc.wantBase)
			}
			if len(labels) != 1 || labels[0].Key != "envoy_rbac_prefix" || labels[0].Value != tc.wantPfx {
				t.Errorf("labels: got %v, want [envoy_rbac_prefix=%s]", labels, tc.wantPfx)
			}
		})
	}
}

func TestFlattenToProm_NetworkRBAC_ShadowRulesStatPrefix(t *testing.T) {
	// POSITIVE (shadow_rules_stat_prefix): the optional middle segment between
	// `rbac.` and the shadow_* counter is preserved in the flattened base name
	// (dots→underscores), and the outer stat_prefix is still extracted as the
	// label. Per SPEC §7.1.
	//
	//   rbac_shadow.rbac.myshadow.shadow_allowed →
	//       envoy_rbac_myshadow_shadow_allowed{envoy_rbac_prefix="rbac_shadow"}
	base, labels, err := flattenToProm("rbac_shadow.rbac.myshadow.shadow_allowed")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	wantBase := "envoy_rbac_myshadow_shadow_allowed"
	if base != wantBase {
		t.Errorf("base: got %q, want %q", base, wantBase)
	}
	if len(labels) != 1 || labels[0].Key != "envoy_rbac_prefix" || labels[0].Value != "rbac_shadow" {
		t.Errorf("labels: got %v, want [envoy_rbac_prefix=rbac_shadow]", labels)
	}
}

func TestFlattenToProm_NetworkRBAC_ShadowRulesStatPrefix_Denied(t *testing.T) {
	// POSITIVE (shadow_rules_stat_prefix with shadow_denied counter).
	//   sp.rbac.ns.shadow_denied → envoy_rbac_ns_shadow_denied{envoy_rbac_prefix="sp"}
	base, labels, err := flattenToProm("sp.rbac.ns.shadow_denied")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	wantBase := "envoy_rbac_ns_shadow_denied"
	if base != wantBase {
		t.Errorf("base: got %q, want %q", base, wantBase)
	}
	if len(labels) != 1 || labels[0].Key != "envoy_rbac_prefix" || labels[0].Value != "sp" {
		t.Errorf("labels: got %v, want [envoy_rbac_prefix=sp]", labels)
	}
}

func TestFlattenToProm_NetworkRBAC_RejectsUnknownCounter(t *testing.T) {
	// REJECT unknown counter: `foo.rbac.bogus` does NOT match the rbac rule
	// (suffix check rejects "rbac.bogus") and falls through to the error return.
	// Guard: the strings.HasSuffix allowlist in the rbacSegment block.
	_, _, err := flattenToProm("foo.rbac.bogus")
	if err == nil {
		t.Error("flattenToProm with unknown rbac counter: want error, got nil")
	}
}

func TestFlattenToProm_NetworkRBAC_RejectsLeadingDot(t *testing.T) {
	// REJECT leading dot: `.rbac.allowed` — the idx>0 guard rejects idx==0 so
	// prefix can never be empty. The name falls through to the error return.
	// Guard: `idx > 0` in the rbacSegment block.
	_, _, err := flattenToProm(".rbac.allowed")
	if err == nil {
		t.Error("flattenToProm with leading-dot prefix: want error, got nil (idx==0 must reject)")
	}
}

func TestFlattenToProm_NetworkRBAC_RejectsDottedPrefix(t *testing.T) {
	// REJECT dotted prefix: `a.b.rbac.allowed` — the first occurrence of
	// `.rbac.` is at position 1, giving prefix="a" and the suffix check passes,
	// BUT strings.Index on "a.b.rbac.allowed" finds `.rbac.` at index 3, giving
	// prefix="a.b" which ContainsRune('.') → the no-dot-prefix guard rejects it.
	// Confirm it does NOT produce envoy_rbac_allowed.
	// Guard: `!strings.ContainsRune(prefix, '.')` in the rbacSegment block.
	_, _, err := flattenToProm("a.b.rbac.allowed")
	if err == nil {
		t.Error("flattenToProm with dotted prefix: want error, got nil (dotted stat_prefix must reject)")
	}
}

func TestFlattenToProm_NetworkRBAC_DoesNotConflictWithSN1234(t *testing.T) {
	// NON-CONFLICT with SN1 (cluster.): a name starting with `cluster.` enters
	// the SN1 case before the default branch, so the rbac rule is never reached.
	// Load-bearing: SN1 wins → base starts with envoy_cluster_, NOT envoy_rbac_.
	base, labels, err := flattenToProm("cluster.foo.rbac.allowed")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	if !strings.HasPrefix(base, "envoy_cluster_") {
		t.Errorf("base: got %q, want SN1-prefixed envoy_cluster_* (rbac rule must not fire for cluster. names)", base)
	}
	if len(labels) == 0 || labels[0].Key != "envoy_cluster_name" {
		t.Errorf("labels: got %v, want envoy_cluster_name=foo (SN1 wins)", labels)
	}
}

func TestFlattenToProm_NetworkRBAC_DoesNotConflictWithSN2_HTTP(t *testing.T) {
	// NON-CONFLICT with SN2 (http.): the HTTP rbac filter's stats flow through
	// the SN2 `http.<hcm>.rbac.*` path (dots→underscores) and NEVER reach the
	// default branch. Confirm that `http.ingress.rbac.allowed` produces the
	// SN2 HTTP shape (envoy_http_rbac_allowed), NOT envoy_rbac_allowed.
	base, labels, err := flattenToProm("http.ingress.rbac.allowed")
	if err != nil {
		t.Fatalf("flattenToProm: %v", err)
	}
	wantBase := "envoy_http_rbac_allowed"
	if base != wantBase {
		t.Errorf("base: got %q, want %q (SN2 wins; rbac rule must not fire for http. names)", base, wantBase)
	}
	if len(labels) == 0 || labels[0].Key != "envoy_http_conn_manager_prefix" {
		t.Errorf("labels: got %v, want envoy_http_conn_manager_prefix=ingress (SN2 wins)", labels)
	}
}
