package accesslog

import (
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
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

func TestGrpcSinkCounters(t *testing.T) {
	reg := stats.NewRegistry()

	// Count metrics before registration (delta assertion).
	var before []string
	reg.Walk(func(m stats.Metric) { before = append(before, m.Name()) })

	written, dropped := RegisterGrpcSinkCounters(reg)

	// Both counters must be non-nil.
	if written == nil {
		t.Fatal("RegisterGrpcSinkCounters: written is nil")
	}
	if dropped == nil {
		t.Fatal("RegisterGrpcSinkCounters: dropped is nil")
	}

	// Counters must be distinct pointers.
	if written == dropped {
		t.Fatal("RegisterGrpcSinkCounters: written and dropped are the same pointer")
	}

	// Verify the expected static names.
	if written.Name() != "access_logs.grpc_access_log.logs_written" {
		t.Errorf("written name = %q, want access_logs.grpc_access_log.logs_written", written.Name())
	}
	if dropped.Name() != "access_logs.grpc_access_log.logs_dropped" {
		t.Errorf("dropped name = %q, want access_logs.grpc_access_log.logs_dropped", dropped.Name())
	}

	// Surface delta must be exactly +2.
	var after []string
	reg.Walk(func(m stats.Metric) { after = append(after, m.Name()) })
	delta := len(after) - len(before)
	if delta != 2 {
		t.Errorf("registration delta = %d, want 2 (before=%v, after=%v)", delta, before, after)
	}

	// Both names must appear in the registry after registration.
	nameSet := make(map[string]bool, len(after))
	for _, n := range after {
		nameSet[n] = true
	}
	if !nameSet["access_logs.grpc_access_log.logs_written"] {
		t.Errorf("registry missing access_logs.grpc_access_log.logs_written; got %v", after)
	}
	if !nameSet["access_logs.grpc_access_log.logs_dropped"] {
		t.Errorf("registry missing access_logs.grpc_access_log.logs_dropped; got %v", after)
	}
}

func TestOTLPSinkCounters(t *testing.T) {
	reg := stats.NewRegistry()

	// Count metrics before registration (delta assertion).
	var before []string
	reg.Walk(func(m stats.Metric) { before = append(before, m.Name()) })

	written, dropped := RegisterOTLPSinkCounters(reg)

	// Both counters must be non-nil.
	if written == nil {
		t.Fatal("RegisterOTLPSinkCounters: written is nil")
	}
	if dropped == nil {
		t.Fatal("RegisterOTLPSinkCounters: dropped is nil")
	}

	// Counters must be distinct pointers.
	if written == dropped {
		t.Fatal("RegisterOTLPSinkCounters: written and dropped are the same pointer")
	}

	// Verify the expected static names.
	if written.Name() != "access_logs.open_telemetry_access_log.logs_written" {
		t.Errorf("written name = %q, want access_logs.open_telemetry_access_log.logs_written", written.Name())
	}
	if dropped.Name() != "access_logs.open_telemetry_access_log.logs_dropped" {
		t.Errorf("dropped name = %q, want access_logs.open_telemetry_access_log.logs_dropped", dropped.Name())
	}

	// Surface delta must be exactly +2.
	var after []string
	reg.Walk(func(m stats.Metric) { after = append(after, m.Name()) })
	delta := len(after) - len(before)
	if delta != 2 {
		t.Errorf("registration delta = %d, want 2 (before=%v, after=%v)", delta, before, after)
	}

	// Both names must appear in the registry after registration.
	nameSet := make(map[string]bool, len(after))
	for _, n := range after {
		nameSet[n] = true
	}
	if !nameSet["access_logs.open_telemetry_access_log.logs_written"] {
		t.Errorf("registry missing access_logs.open_telemetry_access_log.logs_written; got %v", after)
	}
	if !nameSet["access_logs.open_telemetry_access_log.logs_dropped"] {
		t.Errorf("registry missing access_logs.open_telemetry_access_log.logs_dropped; got %v", after)
	}
}
