package accesslog

import (
	"testing"
	"time"
)

func TestRecord_AllFieldsZeroValueWellDefined(t *testing.T) {
	var r Record
	if !r.StartTime.IsZero() { t.Errorf("StartTime zero-value not zero: %v", r.StartTime) }
	if r.Method != "" { t.Errorf("Method zero-value not empty: %q", r.Method) }
	if r.Path != "" { t.Errorf("Path zero-value not empty: %q", r.Path) }
	if r.Protocol != "" { t.Errorf("Protocol zero-value not empty: %q", r.Protocol) }
	if r.ResponseCode != 0 { t.Errorf("ResponseCode zero-value not 0: %d", r.ResponseCode) }
	if r.BytesSent != 0 { t.Errorf("BytesSent zero-value not 0: %d", r.BytesSent) }
	if r.Duration != 0 { t.Errorf("Duration zero-value not 0: %v", r.Duration) }
	if r.Authority != "" { t.Errorf("Authority zero-value not empty: %q", r.Authority) }
	if r.UserAgent != "" { t.Errorf("UserAgent zero-value not empty: %q", r.UserAgent) }
	if r.UpstreamHost != "" { t.Errorf("UpstreamHost zero-value not empty: %q", r.UpstreamHost) }
}

func TestRecord_PopulatedShape(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 34, 56, 789000000, time.UTC)
	r := Record{
		StartTime: now, Method: "GET", Path: "/health", Protocol: "HTTP/1.1",
		ResponseCode: 200, BytesSent: 3, Duration: 5 * time.Millisecond,
		Authority: "127.0.0.1:10000", UserAgent: "Go-http-client/1.1",
		UpstreamHost: "10.0.0.1:8080",
	}
	if r.ResponseCode != 200 { t.Errorf("ResponseCode = %d, want 200", r.ResponseCode) }
	if r.Duration != 5*time.Millisecond { t.Errorf("Duration = %v, want 5ms", r.Duration) }
}

// captureSink is a test double for the Sink interface; used by Tasks 10/12/13.
type captureSink struct{ recs []*Record }
func (s *captureSink) Submit(r *Record)  { s.recs = append(s.recs, r) }
func (s *captureSink) Close() error      { return nil }

func TestSink_InterfaceImplementation(t *testing.T) {
	var s Sink = &captureSink{}
	r := &Record{Method: "GET", Path: "/x", ResponseCode: 200}
	s.Submit(r)
	if err := s.Close(); err != nil { t.Errorf("Close() error: %v", err) }
	cs := s.(*captureSink)
	if len(cs.recs) != 1 { t.Fatalf("captured %d records, want 1", len(cs.recs)) }
	if cs.recs[0].Method != "GET" { t.Errorf("captured Method = %q, want GET", cs.recs[0].Method) }
}
