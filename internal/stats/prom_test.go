package stats

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteProm_EmptyRegistry(t *testing.T) {
	r := NewRegistry()
	var buf bytes.Buffer
	if err := WriteProm(&buf, r); err != nil {
		t.Fatalf("WriteProm: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("WriteProm on empty registry produced %d bytes, want 0", buf.Len())
	}
}

func TestWriteProm_SingleCounter(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("listener.0_0_0_0_10000.downstream_cx_total")
	c.Add(42)
	var buf bytes.Buffer
	if err := WriteProm(&buf, r); err != nil {
		t.Fatalf("WriteProm: %v", err)
	}
	out := buf.String()
	wantHelp := "# HELP envoy_listener_downstream_cx_total Total connections accepted on the listener."
	wantType := "# TYPE envoy_listener_downstream_cx_total counter"
	wantLine := `envoy_listener_downstream_cx_total{envoy_listener_address="0_0_0_0_10000"} 42`
	for _, w := range []string{wantHelp, wantType, wantLine} {
		if !strings.Contains(out, w) {
			t.Errorf("WriteProm output missing line %q\n--- output ---\n%s", w, out)
		}
	}
}

func TestWriteProm_StatusClassCollapse(t *testing.T) {
	r := NewRegistry()
	for digit := 2; digit <= 5; digit++ {
		c := r.NewCounter("cluster.c0.upstream_rq_" + string(rune('0'+digit)) + "xx")
		c.Add(uint64(digit))
	}
	var buf bytes.Buffer
	if err := WriteProm(&buf, r); err != nil {
		t.Fatalf("WriteProm: %v", err)
	}
	out := buf.String()
	// Exactly one HELP + one TYPE for the collapsed group.
	if got := strings.Count(out, "# HELP envoy_cluster_upstream_rq_xx"); got != 1 {
		t.Errorf("# HELP envoy_cluster_upstream_rq_xx count = %d, want 1\n--- output ---\n%s", got, out)
	}
	if got := strings.Count(out, "# TYPE envoy_cluster_upstream_rq_xx counter"); got != 1 {
		t.Errorf("# TYPE envoy_cluster_upstream_rq_xx count = %d, want 1\n--- output ---\n%s", got, out)
	}
	// Four metric lines with each class digit value.
	for digit := 2; digit <= 5; digit++ {
		want := `envoy_cluster_upstream_rq_xx{envoy_response_code_class="` + string(rune('0'+digit)) + `",envoy_cluster_name="c0"} ` + string(rune('0'+digit))
		if !strings.Contains(out, want) {
			t.Errorf("WriteProm output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestWriteProm_AlphabeticallySortedGroups(t *testing.T) {
	r := NewRegistry()
	// Register intentionally out of alphabetical order.
	r.NewCounter("cluster.c0.upstream_cx_total")
	r.NewGauge("server.live").Set(1)
	r.NewCounter("listener.0_0_0_0_10000.downstream_cx_total")
	var buf bytes.Buffer
	if err := WriteProm(&buf, r); err != nil {
		t.Fatalf("WriteProm: %v", err)
	}
	out := buf.String()
	clusterIdx := strings.Index(out, "# HELP envoy_cluster_upstream_cx_total")
	listenerIdx := strings.Index(out, "# HELP envoy_listener_downstream_cx_total")
	serverIdx := strings.Index(out, "# HELP envoy_server_live")
	if clusterIdx < 0 || listenerIdx < 0 || serverIdx < 0 {
		t.Fatalf("missing groups: cluster=%d listener=%d server=%d\n--- output ---\n%s", clusterIdx, listenerIdx, serverIdx, out)
	}
	if !(clusterIdx < listenerIdx && listenerIdx < serverIdx) {
		t.Errorf("groups not alphabetically sorted: cluster@%d listener@%d server@%d\n--- output ---\n%s", clusterIdx, listenerIdx, serverIdx, out)
	}
}

func TestWriteProm_GaugeRendersNegative(t *testing.T) {
	r := NewRegistry()
	g := r.NewGauge("listener.0_0_0_0_10000.downstream_cx_active")
	g.Set(-3)
	var buf bytes.Buffer
	if err := WriteProm(&buf, r); err != nil {
		t.Fatalf("WriteProm: %v", err)
	}
	out := buf.String()
	want := "} -3"
	if !strings.Contains(out, want) {
		t.Errorf("WriteProm output missing %q (negative gauge value)\n--- output ---\n%s", want, out)
	}
}

type synthMetric struct {
	name   string
	mtype  MetricType
	format string
}

func (s *synthMetric) Name() string     { return s.name }
func (s *synthMetric) Type() MetricType { return s.mtype }
func (s *synthMetric) Format() string   { return s.format }

func TestWriteProm_EscapesLabelValues(t *testing.T) {
	r := NewRegistry()
	// Synthetic Metric injection — bypasses NewCounter's nameRE which would
	// reject the adversarial address. The writer's escaping path is tested
	// independently of the register-time validation.
	r.metrics = append(r.metrics, &synthMetric{
		name:   `listener.weird"addr.downstream_cx_total`,
		mtype:  MetricCounter,
		format: "1",
	})
	r.byName[`listener.weird"addr.downstream_cx_total`] = r.metrics[0]
	var buf bytes.Buffer
	if err := WriteProm(&buf, r); err != nil {
		t.Fatalf("WriteProm: %v", err)
	}
	if !strings.Contains(buf.String(), `envoy_listener_address="weird\"addr"`) {
		t.Errorf("WriteProm did not escape double-quote in label value\n--- output ---\n%s", buf.String())
	}
}
