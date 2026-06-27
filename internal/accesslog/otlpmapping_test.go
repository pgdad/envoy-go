package accesslog

import (
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
)

// strVal is a tiny helper building a string-valued AnyValue (test-local).
func strVal(s string) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: s}}
}

// mustCompileValue compiles an AnyValue tree into an OTLPValueTemplate, failing the
// test on a compile error.
func mustCompileValue(t *testing.T, v *commonpb.AnyValue) *OTLPValueTemplate {
	t.Helper()
	tmpl, err := CompileOTLPValue(v)
	if err != nil {
		t.Fatalf("CompileOTLPValue(%v): %v", v, err)
	}
	return tmpl
}

// TestOTLPMappingBuildLogRecord — the built-in path (body==nil && len(attrs)==0)
// carries ONLY time_unix_nano (byte-identical to 45.1); a non-nil body/attrs adds
// the operator-templated body + LogRecord.attributes (45.2 operator templating).
func TestOTLPMappingBuildLogRecord(t *testing.T) {
	ts := time.Date(2026, 6, 26, 1, 2, 3, 456789000, time.UTC)

	t.Run("builtin-lean-record", func(t *testing.T) {
		rec := &Record{StartTime: ts}
		lr := buildLogRecord(rec, nil, nil)

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
	})

	t.Run("string-body-plus-one-attr", func(t *testing.T) {
		rec := &Record{StartTime: ts, Method: "GET", ResponseCode: 200}
		bodyTmpl := mustCompileValue(t, strVal("%REQ(:METHOD)% %RESPONSE_CODE%"))
		attrTmpls := []OTLPAttrTemplate{
			{Key: "m", Value: mustCompileValue(t, strVal("%REQ(:METHOD)%"))},
		}
		lr := buildLogRecord(rec, bodyTmpl, attrTmpls)

		if got, want := lr.GetTimeUnixNano(), uint64(ts.UnixNano()); got != want {
			t.Errorf("TimeUnixNano = %d, want %d (still set)", got, want)
		}
		if got, want := lr.GetBody().GetStringValue(), "GET 200"; got != want {
			t.Errorf("Body.StringValue = %q, want %q", got, want)
		}
		attrs := lr.GetAttributes()
		if len(attrs) != 1 {
			t.Fatalf("len(Attributes) = %d, want 1", len(attrs))
		}
		if got := attrs[0].GetKey(); got != "m" {
			t.Errorf("Attributes[0].Key = %q, want %q", got, "m")
		}
		if got := attrs[0].GetValue().GetStringValue(); got != "GET" {
			t.Errorf("Attributes[0].StringValue = %q, want %q", got, "GET")
		}
	})

	t.Run("nested-kvlist-body", func(t *testing.T) {
		rec := &Record{StartTime: ts, Method: "GET"}
		nested := &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{
			Values: []*commonpb.KeyValue{{Key: "a", Value: strVal("%REQ(:METHOD)%")}},
		}}}
		bodyTmpl := mustCompileValue(t, nested)
		lr := buildLogRecord(rec, bodyTmpl, nil)

		kvs := lr.GetBody().GetKvlistValue().GetValues()
		if len(kvs) != 1 {
			t.Fatalf("len(Body.KvlistValue) = %d, want 1", len(kvs))
		}
		if got := kvs[0].GetKey(); got != "a" {
			t.Errorf("Body.KvlistValue[0].Key = %q, want %q", got, "a")
		}
		if got := kvs[0].GetValue().GetStringValue(); got != "GET" {
			t.Errorf("Body.KvlistValue[0].StringValue = %q, want %q", got, "GET")
		}
	})
}

// TestOTLPMappingBuildResource — the 4 built-in Resource labels (dropped wholesale by
// disable_builtin_labels), with the literal resource_attrs ALWAYS appended after the
// built-ins and SURVIVING disable_builtin_labels (AMEND-OPS-1/5). resource_attrs are
// VERBATIM strings (no operator substitution).
func TestOTLPMappingBuildResource(t *testing.T) {
	type kv struct{ k, v string }
	resAttrs := []*commonpb.KeyValue{
		{Key: "svc", Value: strVal("x")},
		{Key: "auth", Value: strVal("%REQ(:AUTHORITY)%")},
	}
	tests := []struct {
		name      string
		node      *corev3.Node
		logName   string
		disable   bool
		resAttrs  []*commonpb.KeyValue
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
			name:      "disable-builtin-labels-no-resattrs-empty",
			node:      &corev3.Node{Id: "n", Cluster: "c", Locality: &corev3.Locality{Zone: "z"}},
			logName:   "L",
			disable:   true,
			wantEmpty: true,
		},
		{
			name:     "builtins-then-resource-attrs",
			node:     &corev3.Node{Id: "n", Cluster: "c", Locality: &corev3.Locality{Zone: "z"}},
			logName:  "L",
			resAttrs: resAttrs,
			wantKVs: []kv{
				{"log_name", "L"},
				{"zone_name", "z"},
				{"cluster_name", "c"},
				{"node_name", "n"},
				{"svc", "x"},
				{"auth", "%REQ(:AUTHORITY)%"}, // VERBATIM — no operator substitution
			},
		},
		{
			name:     "disable-builtins-resource-attrs-survive",
			node:     &corev3.Node{Id: "n", Cluster: "c", Locality: &corev3.Locality{Zone: "z"}},
			logName:  "L",
			disable:  true,
			resAttrs: resAttrs,
			wantKVs: []kv{
				{"svc", "x"},
				{"auth", "%REQ(:AUTHORITY)%"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := buildResource(tc.node, tc.logName, tc.disable, tc.resAttrs)
			attrs := res.GetAttributes()
			if tc.wantEmpty {
				if len(attrs) != 0 {
					t.Fatalf("Attributes = %v, want empty (disable_builtin_labels, no resource_attrs)", attrs)
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

	// resource_attrs are shared-immutable: buildResource must NOT mutate them. Assert
	// the input slice/objects survive two calls unchanged.
	t.Run("resource-attrs-not-mutated", func(t *testing.T) {
		node := &corev3.Node{Id: "n", Cluster: "c", Locality: &corev3.Locality{Zone: "z"}}
		shared := []*commonpb.KeyValue{{Key: "svc", Value: strVal("x")}}
		_ = buildResource(node, "L", false, shared)
		_ = buildResource(node, "L", false, shared)
		if len(shared) != 1 || shared[0].GetKey() != "svc" || shared[0].GetValue().GetStringValue() != "x" {
			t.Errorf("shared resource_attrs mutated: %v", shared)
		}
	})
}

// TestOTLPMappingBuildExportRequest — one ResourceLogs (Resource = the built-in labels
// THEN the literal resource_attrs) → one ScopeLogs (Scope ABSENT) → the already-built
// batch in order. buildExportRequest takes an ALREADY-BUILT batch (per-record body/attr
// Eval happens in buildLogRecord upstream).
func TestOTLPMappingBuildExportRequest(t *testing.T) {
	node := &corev3.Node{Id: "n", Cluster: "c", Locality: &corev3.Locality{Zone: "z"}}
	resAttrs := []*commonpb.KeyValue{
		{Key: "svc", Value: strVal("x")},
		{Key: "auth", Value: strVal("%REQ(:AUTHORITY)%")},
	}
	t0 := time.Date(2026, 6, 26, 0, 0, 0, 1, time.UTC)
	t1 := time.Date(2026, 6, 26, 0, 0, 0, 2, time.UTC)
	batch := []*logspb.LogRecord{
		buildLogRecord(&Record{StartTime: t0}, nil, nil),
		buildLogRecord(&Record{StartTime: t1}, nil, nil),
	}
	req := buildExportRequest(batch, node, "L", false, resAttrs)

	rls := req.GetResourceLogs()
	if len(rls) != 1 {
		t.Fatalf("len(ResourceLogs) = %d, want 1", len(rls))
	}
	rl := rls[0]

	// Resource == buildResource(...): the 4 built-ins THEN the resource_attrs.
	wantRes := buildResource(node, "L", false, resAttrs)
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
