package buffer

import (
	"strings"
	"testing"

	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// --- Group 1: New factory PGV-mirror ---

func TestNew_NilTC(t *testing.T) {
	_, err := New(nil, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("expected error on nil typed_config")
	}
	if !strings.Contains(err.Error(), "buffer:") {
		t.Errorf("error wording missing 'buffer:' prefix: %v", err)
	}
}

func TestNew_MalformedTC(t *testing.T) {
	any := &anypb.Any{TypeUrl: TypeURL, Value: []byte("not-a-valid-proto")}
	_, err := New(any, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("expected error on malformed typed_config")
	}
}

func TestNew_MaxRequestBytesNil_RejectAtParseTime(t *testing.T) {
	cfg := &bufferv3.Buffer{} // MaxRequestBytes nil
	any := mustMarshalAny(t, cfg)
	_, err := New(any, envoyhttp.FactoryCtx{})
	if err == nil || !strings.Contains(err.Error(), "max_request_bytes is required") {
		t.Errorf("expected 'max_request_bytes is required' error, got: %v", err)
	}
}

func TestNew_MaxRequestBytesZero_RejectAtParseTime(t *testing.T) {
	cfg := &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(0)}
	any := mustMarshalAny(t, cfg)
	_, err := New(any, envoyhttp.FactoryCtx{})
	if err == nil || !strings.Contains(err.Error(), "must be > 0") {
		t.Errorf("expected 'must be > 0' error, got: %v", err)
	}
}

func TestNew_MaxRequestBytesOverCap_RejectAtParseTime(t *testing.T) {
	cases := []uint32{1048577, 2 * 1024 * 1024, 5 * 1024 * 1024}
	for _, v := range cases {
		v := v
		t.Run("", func(t *testing.T) {
			cfg := &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(v)}
			any := mustMarshalAny(t, cfg)
			_, err := New(any, envoyhttp.FactoryCtx{})
			if err == nil || !strings.Contains(err.Error(), "exceeds envoy-go cap of 1048576 bytes") {
				t.Errorf("expected over-cap error for v=%d, got: %v", v, err)
			}
		})
	}
}

func TestNew_MaxRequestBytesBoundary_Accepted(t *testing.T) {
	cases := []uint32{1, 65536, 1048576}
	for _, v := range cases {
		v := v
		t.Run("", func(t *testing.T) {
			cfg := &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(v)}
			any := mustMarshalAny(t, cfg)
			factory, err := New(any, envoyhttp.FactoryCtx{})
			if err != nil {
				t.Fatalf("expected accept for v=%d, got error: %v", v, err)
			}
			if factory == nil {
				t.Fatal("expected non-nil factory")
			}
		})
	}
}

func TestNew_HappyPath_Round(t *testing.T) {
	cfg := &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(1024)}
	any := mustMarshalAny(t, cfg)
	factory, err := New(any, envoyhttp.FactoryCtx{})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	hf := factory()
	if hf.Decoder == nil || hf.Encoder != nil {
		t.Errorf("expected decoder-only HTTPFilter (Decoder!=nil, Encoder==nil), got %+v", hf)
	}
}

// --- Group 2: parsePerRoute PGV-mirror discipline ---

func TestParsePerRoute_Disabled_Parses(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Disabled{Disabled: true}}
	cpr, err := parsePerRoute(pr)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if !cpr.disabled || cpr.maxOverride != nil {
		t.Errorf("expected disabled=true, maxOverride=nil; got %+v", cpr)
	}
}

func TestParsePerRoute_BufferOverride_Parses(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Buffer{Buffer: &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(65536)}}}
	cpr, err := parsePerRoute(pr)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if cpr.disabled || cpr.maxOverride == nil || *cpr.maxOverride != 65536 {
		t.Errorf("expected disabled=false, maxOverride=&65536; got %+v", cpr)
	}
}

func TestParsePerRoute_BufferOverride_Zero_Rejects(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Buffer{Buffer: &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(0)}}}
	_, err := parsePerRoute(pr)
	if err == nil || !strings.Contains(err.Error(), "must be > 0") {
		t.Errorf("expected zero-rejection, got: %v", err)
	}
}

func TestParsePerRoute_BufferOverride_OverCap_Rejects(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Buffer{Buffer: &bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(5 * 1024 * 1024)}}}
	_, err := parsePerRoute(pr)
	if err == nil || !strings.Contains(err.Error(), "exceeds envoy-go cap") {
		t.Errorf("expected over-cap rejection, got: %v", err)
	}
}

func TestParsePerRoute_OneofUnset_Rejects(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{} // Override nil
	_, err := parsePerRoute(pr)
	if err == nil || !strings.Contains(err.Error(), "override oneof is required") {
		t.Errorf("expected oneof-required rejection, got: %v", err)
	}
}

func TestParsePerRoute_DisabledFalse_Rejects(t *testing.T) {
	pr := &bufferv3.BufferPerRoute{Override: &bufferv3.BufferPerRoute_Disabled{Disabled: false}}
	_, err := parsePerRoute(pr)
	if err == nil || !strings.Contains(err.Error(), "disabled must be true") {
		t.Errorf("expected disabled-bool.const rejection, got: %v", err)
	}
}

// --- Helpers ---

func mustMarshalAny(t *testing.T, m proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}
