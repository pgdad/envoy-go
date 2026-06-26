package accesslog

import (
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
)

// TestOTLPMappingBuildLogRecord — the LEAN built-in OTLP LogRecord carries ONLY
// time_unix_nano (SPEC §11 live probe vs contrib-v1.37.2): no observed_time,
// severity, body, or LogRecord.attributes.
func TestOTLPMappingBuildLogRecord(t *testing.T) {
	ts := time.Date(2026, 6, 26, 1, 2, 3, 456789000, time.UTC)
	rec := &Record{StartTime: ts}
	lr := buildLogRecord(rec)

	if got, want := lr.GetTimeUnixNano(), uint64(ts.UnixNano()); got != want {
		t.Errorf("TimeUnixNano = %d, want %d", got, want)
	}
	if got := lr.GetObservedTimeUnixNano(); got != 0 {
		t.Errorf("ObservedTimeUnixNano = %d, want 0 (lean built-in record)", got)
	}
	if got := lr.GetSeverityNumber(); got != 0 {
		t.Errorf("SeverityNumber = %v, want 0 (lean built-in record)", got)
	}
	if got := lr.GetSeverityText(); got != "" {
		t.Errorf("SeverityText = %q, want \"\" (lean built-in record)", got)
	}
	if lr.GetBody() != nil {
		t.Errorf("Body = %v, want nil (lean built-in record)", lr.GetBody())
	}
	if got := lr.GetAttributes(); len(got) != 0 {
		t.Errorf("Attributes = %v, want empty (lean built-in record)", got)
	}
}

// TestOTLPMappingBuildResource — the 4 built-in Resource labels, always all 4
// keys in this fixed order, dropped wholesale by disable_builtin_labels.
func TestOTLPMappingBuildResource(t *testing.T) {
	type kv struct{ k, v string }
	tests := []struct {
		name      string
		node      *corev3.Node
		logName   string
		disable   bool
		wantKVs   []kv
		wantEmpty bool
	}{
		{
			name:    "populated",
			node:    &corev3.Node{Id: "n", Cluster: "c", Locality: &corev3.Locality{Zone: "z"}},
			logName: "L",
			wantKVs: []kv{
				{"log_name", "L"},
				{"zone_name", "z"},
				{"cluster_name", "c"},
				{"node_name", "n"},
			},
		},
		{
			name:    "empty-node-empty-logname-still-4",
			node:    &corev3.Node{},
			logName: "",
			wantKVs: []kv{
				{"log_name", ""},
				{"zone_name", ""},
				{"cluster_name", ""},
				{"node_name", ""},
			},
		},
		{
			name:      "disable-builtin-labels-empty",
			node:      &corev3.Node{Id: "n", Cluster: "c", Locality: &corev3.Locality{Zone: "z"}},
			logName:   "L",
			disable:   true,
			wantEmpty: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := buildResource(tc.node, tc.logName, tc.disable)
			attrs := res.GetAttributes()
			if tc.wantEmpty {
				if len(attrs) != 0 {
					t.Fatalf("Attributes = %v, want empty (disable_builtin_labels)", attrs)
				}
				return
			}
			if len(attrs) != len(tc.wantKVs) {
				t.Fatalf("len(Attributes) = %d, want %d", len(attrs), len(tc.wantKVs))
			}
			for i, want := range tc.wantKVs {
				if got := attrs[i].GetKey(); got != want.k {
					t.Errorf("Attributes[%d].Key = %q, want %q", i, got, want.k)
				}
				if got := attrs[i].GetValue().GetStringValue(); got != want.v {
					t.Errorf("Attributes[%d].StringValue = %q, want %q", i, got, want.v)
				}
			}
		})
	}
}

// TestOTLPMappingBuildExportRequest — one ResourceLogs (Resource = the built-in
// labels) → one ScopeLogs (Scope ABSENT) → the batch in order.
func TestOTLPMappingBuildExportRequest(t *testing.T) {
	node := &corev3.Node{Id: "n", Cluster: "c", Locality: &corev3.Locality{Zone: "z"}}
	t0 := time.Date(2026, 6, 26, 0, 0, 0, 1, time.UTC)
	t1 := time.Date(2026, 6, 26, 0, 0, 0, 2, time.UTC)
	batch := []*logspb.LogRecord{
		buildLogRecord(&Record{StartTime: t0}),
		buildLogRecord(&Record{StartTime: t1}),
	}
	req := buildExportRequest(batch, node, "L", false)

	rls := req.GetResourceLogs()
	if len(rls) != 1 {
		t.Fatalf("len(ResourceLogs) = %d, want 1", len(rls))
	}
	rl := rls[0]

	// Resource == buildResource(...): assert the 4 built-in labels carried through.
	wantRes := buildResource(node, "L", false)
	gotAttrs := rl.GetResource().GetAttributes()
	wantAttrs := wantRes.GetAttributes()
	if len(gotAttrs) != len(wantAttrs) {
		t.Fatalf("Resource attrs len = %d, want %d", len(gotAttrs), len(wantAttrs))
	}
	for i := range wantAttrs {
		if gotAttrs[i].GetKey() != wantAttrs[i].GetKey() ||
			gotAttrs[i].GetValue().GetStringValue() != wantAttrs[i].GetValue().GetStringValue() {
			t.Errorf("Resource attr[%d] = %v, want %v", i, gotAttrs[i], wantAttrs[i])
		}
	}

	sls := rl.GetScopeLogs()
	if len(sls) != 1 {
		t.Fatalf("len(ScopeLogs) = %d, want 1", len(sls))
	}
	sl := sls[0]
	if sl.GetScope() != nil {
		t.Errorf("Scope = %v, want nil (Scope ABSENT)", sl.GetScope())
	}
	gotRecs := sl.GetLogRecords()
	if len(gotRecs) != 2 {
		t.Fatalf("len(LogRecords) = %d, want 2", len(gotRecs))
	}
	if gotRecs[0].GetTimeUnixNano() != uint64(t0.UnixNano()) {
		t.Errorf("LogRecords[0].TimeUnixNano = %d, want %d", gotRecs[0].GetTimeUnixNano(), uint64(t0.UnixNano()))
	}
	if gotRecs[1].GetTimeUnixNano() != uint64(t1.UnixNano()) {
		t.Errorf("LogRecords[1].TimeUnixNano = %d, want %d", gotRecs[1].GetTimeUnixNano(), uint64(t1.UnixNano()))
	}
}
