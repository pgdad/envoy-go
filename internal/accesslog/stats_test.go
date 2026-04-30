package accesslog

import (
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

func TestRegisterDroppedCounter_Name(t *testing.T) {
	reg := stats.NewRegistry()
	c := RegisterDroppedCounter(reg)
	if c == nil {
		t.Fatal("RegisterDroppedCounter returned nil")
	}
	if c.Name() != "server.accesslog_dropped" {
		t.Errorf("counter name = %q, want server.accesslog_dropped", c.Name())
	}
}

func TestRegisterDroppedCounter_FlattensToPromName(t *testing.T) {
	reg := stats.NewRegistry()
	_ = RegisterDroppedCounter(reg)
	var names []string
	reg.Walk(func(m stats.Metric) { names = append(names, m.Name()) })
	if len(names) != 1 || names[0] != "server.accesslog_dropped" {
		t.Errorf("Registry contents = %v, want [server.accesslog_dropped]", names)
	}
}
