package statssink

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
)

func TestLabelMapper_Apply(t *testing.T) {
	lm := newLabelMapper()

	out := lm.apply([]*dto.MetricFamily{
		counterFam("cluster.c_backend.upstream_rq_total", 7),
		gaugeFam("cluster.c0.membership_total", 1),
		counterFam("http.hcm_local.downstream_rq_2xx", 7),
		counterFam("listener_manager.listener_create_success", 1),
	})

	// 1: single-tag Counter — residual name + one label; value unchanged.
	if got := out[0].GetName(); got != "cluster.upstream_rq_total" {
		t.Errorf("counter residual name = %q, want cluster.upstream_rq_total", got)
	}
	if got := out[0].GetMetric()[0].GetCounter().GetValue(); got != 7 {
		t.Errorf("counter value = %v, want 7 (unchanged)", got)
	}
	assertLabels(t, out[0].GetMetric()[0].GetLabel(), [][2]string{{"envoy.cluster_name", "c_backend"}})

	// 2: single-tag Gauge — labels on BOTH types.
	if got := out[1].GetName(); got != "cluster.membership_total" {
		t.Errorf("gauge residual name = %q, want cluster.membership_total", got)
	}
	assertLabels(t, out[1].GetMetric()[0].GetLabel(), [][2]string{{"envoy.cluster_name", "c0"}})

	// 3: SN4 multi-tag — _2xx→_xx + two SORTED labels.
	if got := out[2].GetName(); got != "http.downstream_rq_xx" {
		t.Errorf("2xx residual name = %q, want http.downstream_rq_xx", got)
	}
	assertLabels(t, out[2].GetMetric()[0].GetLabel(), [][2]string{
		{"envoy.http_conn_manager_prefix", "hcm_local"},
		{"envoy.response_code_class", "2"},
	})

	// 4: untagged → full name + empty labels (shared by pointer).
	if got := out[3].GetName(); got != "listener_manager.listener_create_success" {
		t.Errorf("untagged name = %q, want full name unchanged", got)
	}
	if len(out[3].GetMetric()[0].GetLabel()) != 0 {
		t.Errorf("untagged labels = %v, want empty", out[3].GetMetric()[0].GetLabel())
	}
}

func TestLabelMapper_Apply_DoesNotMutateInput(t *testing.T) {
	lm := newLabelMapper()
	in := []*dto.MetricFamily{counterFam("cluster.c_backend.upstream_rq_total", 7)}
	_ = lm.apply(in)
	if got := in[0].GetName(); got != "cluster.c_backend.upstream_rq_total" {
		t.Fatalf("input family name mutated: %q", got)
	}
	if n := len(in[0].GetMetric()[0].GetLabel()); n != 0 {
		t.Fatalf("input metric labels mutated: %d labels", n)
	}
}

// assertLabels checks the LabelPair slice equals want in order (the mapper sorts by key).
func assertLabels(t *testing.T, got []*dto.LabelPair, want [][2]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("labels = %+v, want %v", got, want)
	}
	for i, w := range want {
		if got[i].GetName() != w[0] || got[i].GetValue() != w[1] {
			t.Errorf("label[%d] = {%q,%q}, want {%q,%q}", i, got[i].GetName(), got[i].GetValue(), w[0], w[1])
		}
	}
}
