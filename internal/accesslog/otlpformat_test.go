package accesslog

import (
	"testing"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

// fixedStart is a deterministic StartTime used to assert the %START_TIME% format.
var fixedStart = time.Date(2026, 6, 27, 12, 34, 56, 789_000_000, time.UTC)

func sv(s string) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: s}}
}

func kvl(kvs ...*commonpb.KeyValue) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{Values: kvs}}}
}

func arr(vals ...*commonpb.AnyValue) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: vals}}}
}

func pair(k string, v *commonpb.AnyValue) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: v}
}

func TestOTLPFormatTemplateEval(t *testing.T) {
	rec := &Record{
		StartTime:    fixedStart,
		Method:       "GET",
		Path:         "/health",
		Protocol:     "HTTP/1.1",
		ResponseCode: 200,
		BytesSent:    13,
		Authority:    "a",
		UserAgent:    "ua",
		UpstreamHost: "h",
	}
	cases := []struct {
		name string
		tmpl string
		want string
	}{
		{"multi-operator", "%REQ(:METHOD)% %REQ(:PATH)% %PROTOCOL% %RESPONSE_CODE%", "GET /health HTTP/1.1 200"},
		{"start-time", "%START_TIME%", "2026-06-27T12:34:56.789Z"},
		{"req-method", "%REQ(:METHOD)%", "GET"},
		{"req-path", "%REQ(:PATH)%", "/health"},
		{"req-authority", "%REQ(:AUTHORITY)%", "a"},
		{"req-user-agent", "%REQ(USER-AGENT)%", "ua"},
		{"protocol", "%PROTOCOL%", "HTTP/1.1"},
		{"response-code", "%RESPONSE_CODE%", "200"},
		{"bytes-sent", "%BYTES_SENT%", "13"},
		{"duration", "%DURATION%", "0"},
		{"upstream-host", "%UPSTREAM_HOST%", "h"},
		{"literal-only", "hello world", "hello world"},
		{"percent-literal", "100%% done", "100% done"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpl, err := CompileOTLPTemplate(tc.tmpl)
			if err != nil {
				t.Fatalf("CompileOTLPTemplate(%q) error: %v", tc.tmpl, err)
			}
			got := tmpl.evalString(rec)
			if got != tc.want {
				t.Errorf("evalString(%q) = %q, want %q", tc.tmpl, got, tc.want)
			}
		})
	}
}

func TestOTLPFormatMissingValueDash(t *testing.T) {
	cases := []struct {
		name string
		tmpl string
		rec  *Record
		want string
	}{
		{"empty-user-agent", "%REQ(USER-AGENT)%", &Record{}, "-"},
		{"empty-authority", "%REQ(:AUTHORITY)%", &Record{}, "-"},
		{"empty-upstream-host", "%UPSTREAM_HOST%", &Record{}, "-"},
		{"empty-path", "%REQ(:PATH)%", &Record{}, "-"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpl, err := CompileOTLPTemplate(tc.tmpl)
			if err != nil {
				t.Fatalf("CompileOTLPTemplate(%q) error: %v", tc.tmpl, err)
			}
			if got := tmpl.evalString(tc.rec); got != tc.want {
				t.Errorf("evalString(%q) = %q, want %q", tc.tmpl, got, tc.want)
			}
		})
	}
}

func TestOTLPFormatTemplateReject(t *testing.T) {
	cases := []struct {
		name string
		tmpl string
	}{
		{"unknown-operator", "%FOOBAR%"},
		{"valid-envoy-out-of-record", "%UPSTREAM_CLUSTER%"},
		{"arbitrary-header", "%REQ(X-CUSTOM)%"},
		{"resp-operator", "%RESP(CONTENT-TYPE)%"},
		{"unterminated", "%REQ(:METHOD)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CompileOTLPTemplate(tc.tmpl)
			if err == nil {
				t.Fatalf("CompileOTLPTemplate(%q) = nil error, want reject", tc.tmpl)
			}
		})
	}
}

func TestOTLPFormatValueEval(t *testing.T) {
	rec := &Record{Method: "GET", ResponseCode: 200}

	t.Run("string-value", func(t *testing.T) {
		vt, err := CompileOTLPValue(sv("%REQ(:METHOD)%"))
		if err != nil {
			t.Fatalf("CompileOTLPValue error: %v", err)
		}
		got := vt.Eval(rec)
		if got.GetStringValue() != "GET" {
			t.Errorf("GetStringValue() = %q, want %q", got.GetStringValue(), "GET")
		}
	})

	t.Run("kvlist-value", func(t *testing.T) {
		vt, err := CompileOTLPValue(kvl(pair("a", sv("%REQ(:METHOD)%")), pair("b", sv("lit"))))
		if err != nil {
			t.Fatalf("CompileOTLPValue error: %v", err)
		}
		got := vt.Eval(rec).GetKvlistValue()
		if got == nil {
			t.Fatalf("GetKvlistValue() = nil")
		}
		if len(got.GetValues()) != 2 {
			t.Fatalf("len(values) = %d, want 2", len(got.GetValues()))
		}
		if got.GetValues()[0].GetKey() != "a" || got.GetValues()[0].GetValue().GetStringValue() != "GET" {
			t.Errorf("kv[0] = %v, want a->GET", got.GetValues()[0])
		}
		if got.GetValues()[1].GetKey() != "b" || got.GetValues()[1].GetValue().GetStringValue() != "lit" {
			t.Errorf("kv[1] = %v, want b->lit", got.GetValues()[1])
		}
	})

	t.Run("array-value", func(t *testing.T) {
		vt, err := CompileOTLPValue(arr(sv("%RESPONSE_CODE%"), sv("lit")))
		if err != nil {
			t.Fatalf("CompileOTLPValue error: %v", err)
		}
		got := vt.Eval(rec).GetArrayValue()
		if got == nil {
			t.Fatalf("GetArrayValue() = nil")
		}
		if len(got.GetValues()) != 2 {
			t.Fatalf("len(values) = %d, want 2", len(got.GetValues()))
		}
		if got.GetValues()[0].GetStringValue() != "200" {
			t.Errorf("arr[0] = %q, want 200", got.GetValues()[0].GetStringValue())
		}
		if got.GetValues()[1].GetStringValue() != "lit" {
			t.Errorf("arr[1] = %q, want lit", got.GetValues()[1].GetStringValue())
		}
	})

	t.Run("nested-kvlist", func(t *testing.T) {
		vt, err := CompileOTLPValue(kvl(pair("outer", kvl(pair("inner", sv("%REQ(:METHOD)%"))))))
		if err != nil {
			t.Fatalf("CompileOTLPValue error: %v", err)
		}
		outer := vt.Eval(rec).GetKvlistValue()
		if outer == nil || len(outer.GetValues()) != 1 {
			t.Fatalf("outer kvlist = %v", outer)
		}
		inner := outer.GetValues()[0].GetValue().GetKvlistValue()
		if inner == nil || len(inner.GetValues()) != 1 {
			t.Fatalf("inner kvlist = %v", inner)
		}
		if inner.GetValues()[0].GetValue().GetStringValue() != "GET" {
			t.Errorf("inner leaf = %q, want GET", inner.GetValues()[0].GetValue().GetStringValue())
		}
	})

	t.Run("empty-kvlist", func(t *testing.T) {
		vt, err := CompileOTLPValue(kvl())
		if err != nil {
			t.Fatalf("CompileOTLPValue error: %v", err)
		}
		got := vt.Eval(rec)
		if got.GetKvlistValue() == nil {
			t.Fatalf("GetKvlistValue() = nil, want non-nil empty kvlist (type must be preserved)")
		}
		if len(got.GetKvlistValue().GetValues()) != 0 {
			t.Errorf("len(values) = %d, want 0", len(got.GetKvlistValue().GetValues()))
		}
	})

	t.Run("empty-array", func(t *testing.T) {
		vt, err := CompileOTLPValue(arr())
		if err != nil {
			t.Fatalf("CompileOTLPValue error: %v", err)
		}
		got := vt.Eval(rec)
		if got.GetArrayValue() == nil {
			t.Fatalf("GetArrayValue() = nil, want non-nil empty array (type must be preserved)")
		}
		if len(got.GetArrayValue().GetValues()) != 0 {
			t.Errorf("len(values) = %d, want 0", len(got.GetArrayValue().GetValues()))
		}
	})

	// A KeyValue with an unset value is intentionally tolerated: CompileOTLPValue(nil)
	// returns (nil, nil), so the entry compiles to OTLPAttrTemplate{Value: nil} and Eval
	// emits KeyValue{Key, Value: nil} (no panic — nil-receiver Eval returns nil).
	t.Run("value-less-entry", func(t *testing.T) {
		vt, err := CompileOTLPValue(kvl(pair("k", nil)))
		if err != nil {
			t.Fatalf("CompileOTLPValue(value-less entry) error: %v", err)
		}
		got := vt.Eval(rec).GetKvlistValue()
		if got == nil || len(got.GetValues()) != 1 {
			t.Fatalf("kvlist = %v, want 1 entry", got)
		}
		if got.GetValues()[0].GetKey() != "k" {
			t.Errorf("key = %q, want k", got.GetValues()[0].GetKey())
		}
		if got.GetValues()[0].GetValue() != nil {
			t.Errorf("value = %v, want nil (tolerated value-less entry)", got.GetValues()[0].GetValue())
		}
	})

	t.Run("empty-string-leaf", func(t *testing.T) {
		vt, err := CompileOTLPValue(sv(""))
		if err != nil {
			t.Fatalf("CompileOTLPValue error: %v", err)
		}
		if got := vt.Eval(rec).GetStringValue(); got != "" {
			t.Errorf("GetStringValue() = %q, want empty", got)
		}
	})
}

func TestOTLPFormatValueReject(t *testing.T) {
	cases := []struct {
		name string
		val  *commonpb.AnyValue
	}{
		{"int-value", &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 42}}},
		{"bool-value", &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: true}}},
		{"double-value", &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: 1.5}}},
		{"bytes-value", &commonpb.AnyValue{Value: &commonpb.AnyValue_BytesValue{BytesValue: []byte("x")}}},
		{"nested-int-leaf", kvl(pair("k", &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 7}}))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := CompileOTLPValue(tc.val); err == nil {
				t.Fatalf("CompileOTLPValue(%s) = nil error, want reject", tc.name)
			}
		})
	}
}

func TestOTLPFormatValidate(t *testing.T) {
	t.Run("string-operator-literal", func(t *testing.T) {
		if err := ValidateOTLPValue(sv("%REQ(:AUTHORITY)%")); err != nil {
			t.Errorf("ValidateOTLPValue(operator string) = %v, want nil (not scanned)", err)
		}
	})
	// Load-bearing non-scanning guard (AMEND-OPS-1): resource_attributes strings are
	// opaque literals, so an UNKNOWN operator token must STILL validate nil. This bites
	// if ValidateOTLPValue ever started scanning operators (the valid-operator case
	// above would pass even then; an unknown token would not).
	t.Run("string-unknown-operator-not-scanned", func(t *testing.T) {
		if err := ValidateOTLPValue(sv("%FOOBAR%")); err != nil {
			t.Errorf("ValidateOTLPValue(%q) = %v, want nil (opaque literal, not scanned)", "%FOOBAR%", err)
		}
	})
	t.Run("kvlist", func(t *testing.T) {
		if err := ValidateOTLPValue(kvl(pair("k", sv("v")))); err != nil {
			t.Errorf("ValidateOTLPValue(kvlist) = %v, want nil", err)
		}
	})
	t.Run("int-reject", func(t *testing.T) {
		if err := ValidateOTLPValue(&commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 1}}); err == nil {
			t.Errorf("ValidateOTLPValue(int) = nil, want error")
		}
	})
	t.Run("nested-int-reject", func(t *testing.T) {
		if err := ValidateOTLPValue(kvl(pair("k", &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 1}}))); err == nil {
			t.Errorf("ValidateOTLPValue(nested int) = nil, want error")
		}
	})
}
