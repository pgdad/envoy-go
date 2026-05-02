package tls_inspector

import (
	"strings"
	"testing"

	tls_inspectorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestParseConfigNilReturnsDefault(t *testing.T) {
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatalf("parseConfig(nil): %v", err)
	}
	if cfg.bufferSize != defaultBufferSize {
		t.Errorf("bufferSize: got %d, want %d", cfg.bufferSize, defaultBufferSize)
	}
}

func TestParseConfigDefaultBuffer(t *testing.T) {
	tc, err := anypb.New(&tls_inspectorv3.TlsInspector{})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	cfg, err := parseConfig(tc)
	if err != nil {
		t.Fatalf("parseConfig(empty): %v", err)
	}
	if cfg.bufferSize != defaultBufferSize {
		t.Errorf("bufferSize: got %d, want %d", cfg.bufferSize, defaultBufferSize)
	}
}

func TestParseConfigCustomBufferInRange(t *testing.T) {
	tc, err := anypb.New(&tls_inspectorv3.TlsInspector{
		InitialReadBufferSize: wrapperspb.UInt32(1024),
	})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	cfg, err := parseConfig(tc)
	if err != nil {
		t.Fatalf("parseConfig(1024): %v", err)
	}
	if cfg.bufferSize != 1024 {
		t.Errorf("bufferSize: got %d, want 1024", cfg.bufferSize)
	}
}

func TestParseConfigBufferBelowFloorErrors(t *testing.T) {
	tc, err := anypb.New(&tls_inspectorv3.TlsInspector{
		InitialReadBufferSize: wrapperspb.UInt32(128),
	})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	_, err = parseConfig(tc)
	if err == nil {
		t.Fatal("parseConfig(128): want error, got nil")
	}
	want := "tls_inspector: initial_read_buffer_size 128 below floor 256"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error: got %q, want contains %q", err.Error(), want)
	}
}

func TestParseConfigBufferAboveCapClamps(t *testing.T) {
	tc, err := anypb.New(&tls_inspectorv3.TlsInspector{
		InitialReadBufferSize: wrapperspb.UInt32(999999),
	})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	cfg, err := parseConfig(tc)
	if err != nil {
		t.Fatalf("parseConfig(999999): want clamp without error, got %v", err)
	}
	if cfg.bufferSize != maxBufferSize {
		t.Errorf("bufferSize: got %d, want %d (clamped)", cfg.bufferSize, maxBufferSize)
	}
}

func TestParseConfigEnableJA3SilentlyIgnored(t *testing.T) {
	tc, err := anypb.New(&tls_inspectorv3.TlsInspector{
		EnableJa3Fingerprinting: wrapperspb.Bool(true),
	})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	cfg, err := parseConfig(tc)
	if err != nil {
		t.Fatalf("parseConfig(JA3=true): want silent ignore, got %v", err)
	}
	if cfg.bufferSize != defaultBufferSize {
		t.Errorf("bufferSize: got %d, want %d", cfg.bufferSize, defaultBufferSize)
	}
}
