package stats

import (
	"reflect"
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
