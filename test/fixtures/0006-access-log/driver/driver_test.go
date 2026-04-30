// Package driver tests for fixture 0006-access-log.
//
// Tests cover:
//   - ParseLogLine: round-trip the SPEC §11 empirical scrape lines (verbatim)
//     into 15-tuple LogTuples and assert exact field extraction.
//   - AssertRecord: three-tier matrix applied to synthetic tuples.
package driver

import (
	"fmt"
	"testing"
)

// empiricalLines are the verbatim 5 lines from the SPEC §11 empirical scrape
// (reference Envoy v1.37.2 at SHA c5e8a68e..., captured 2026-04-30 by Task 3).
var empiricalLines = []string{
	`[2026-04-30T09:10:30.856Z] "GET /health HTTP/1.1" 200 - 0 3 0 - "-" "curl/8.5.0" "b66c2c7d-3921-4184-b6c1-6a80dd5e7e8e" "127.0.0.1:15006" "-"`,
	`[2026-04-30T09:10:30.861Z] "GET /api/v1/foo HTTP/1.1" 200 - 0 15 0 0 "-" "curl/8.5.0" "1210434d-5aa4-4a56-a256-3ff6fc989ce5" "127.0.0.1:15006" "192.168.65.2:18443"`,
	`[2026-04-30T09:10:30.865Z] "GET /api/v1/bar HTTP/1.1" 200 - 0 15 0 0 "-" "curl/8.5.0" "c76bd1e7-3f55-4a6b-a3df-f88f00c7250a" "127.0.0.1:15006" "192.168.65.2:18443"`,
	`[2026-04-30T09:10:30.870Z] "GET /api/v1/baz HTTP/1.1" 200 - 0 15 0 0 "-" "curl/8.5.0" "5b25ba00-2be4-4ae6-9693-0ce90609f529" "127.0.0.1:15006" "192.168.65.2:18443"`,
	`[2026-04-30T09:10:30.875Z] "GET /notfound HTTP/1.1" 404 - 0 10 0 - "-" "curl/8.5.0" "5a9c562a-1ebf-4676-a556-bf02f89a0fad" "127.0.0.1:15006" "-"`,
}

func TestParseLogLine_EmpiricalPin(t *testing.T) {
	t.Run("health_record", func(t *testing.T) {
		tuple, err := ParseLogLine(empiricalLines[0])
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if tuple.StartTime != "2026-04-30T09:10:30.856Z" {
			t.Errorf("StartTime: got %q", tuple.StartTime)
		}
		if tuple.Method != "GET" {
			t.Errorf("Method: got %q", tuple.Method)
		}
		if tuple.Path != "/health" {
			t.Errorf("Path: got %q", tuple.Path)
		}
		if tuple.Protocol != "HTTP/1.1" {
			t.Errorf("Protocol: got %q", tuple.Protocol)
		}
		if tuple.ResponseCode != "200" {
			t.Errorf("ResponseCode: got %q", tuple.ResponseCode)
		}
		if tuple.ResponseFlags != "-" {
			t.Errorf("ResponseFlags: got %q", tuple.ResponseFlags)
		}
		if tuple.BytesReceived != "0" {
			t.Errorf("BytesReceived: got %q", tuple.BytesReceived)
		}
		if tuple.BytesSent != "3" {
			t.Errorf("BytesSent: got %q", tuple.BytesSent)
		}
		if tuple.Duration != "0" {
			t.Errorf("Duration: got %q", tuple.Duration)
		}
		if tuple.SvcTime != "-" {
			t.Errorf("SvcTime: got %q", tuple.SvcTime)
		}
		if tuple.XForwardedFor != "-" {
			t.Errorf("XForwardedFor: got %q", tuple.XForwardedFor)
		}
		if tuple.UserAgent != "curl/8.5.0" {
			t.Errorf("UserAgent: got %q", tuple.UserAgent)
		}
		if tuple.XRequestID != "b66c2c7d-3921-4184-b6c1-6a80dd5e7e8e" {
			t.Errorf("XRequestID: got %q", tuple.XRequestID)
		}
		if tuple.Authority != "127.0.0.1:15006" {
			t.Errorf("Authority: got %q", tuple.Authority)
		}
		if tuple.UpstreamHost != "-" {
			t.Errorf("UpstreamHost: got %q", tuple.UpstreamHost)
		}
	})

	t.Run("routed_record", func(t *testing.T) {
		tuple, err := ParseLogLine(empiricalLines[1])
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if tuple.Path != "/api/v1/foo" {
			t.Errorf("Path: got %q", tuple.Path)
		}
		if tuple.BytesSent != "15" {
			t.Errorf("BytesSent: got %q", tuple.BytesSent)
		}
		if tuple.UpstreamHost != "192.168.65.2:18443" {
			t.Errorf("UpstreamHost: got %q", tuple.UpstreamHost)
		}
		if tuple.SvcTime != "0" {
			t.Errorf("SvcTime: got %q (want '0')", tuple.SvcTime)
		}
	})

	t.Run("notfound_record", func(t *testing.T) {
		tuple, err := ParseLogLine(empiricalLines[4])
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if tuple.ResponseCode != "404" {
			t.Errorf("ResponseCode: got %q", tuple.ResponseCode)
		}
		if tuple.BytesSent != "10" {
			t.Errorf("BytesSent: got %q", tuple.BytesSent)
		}
		if tuple.UpstreamHost != "-" {
			t.Errorf("UpstreamHost: got %q", tuple.UpstreamHost)
		}
	})

	t.Run("all_lines_parse", func(t *testing.T) {
		for i, line := range empiricalLines {
			if _, err := ParseLogLine(line); err != nil {
				t.Errorf("line[%d]: %v", i, err)
			}
		}
	})
}

func TestParseLogLine_InvalidInput(t *testing.T) {
	cases := []struct {
		name string
		line string
	}{
		{"empty", ""},
		{"no_brackets", `"GET /health HTTP/1.1" 200 - 0 3 0 - "-" "ua" "-" "auth" "-"`},
		{"truncated", `[2026-04-30T09:10:30.856Z] "GET /health HTTP/1.1" 200`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseLogLine(tc.line); err == nil {
				t.Errorf("expected error for line %q", tc.line)
			}
		})
	}
}

// tbMock is a minimal fixture.TB implementation for unit-testing AssertRecord.
type tbMock struct {
	errors []string
	fatals []string
}

func (m *tbMock) Helper()                   {}
func (m *tbMock) Errorf(f string, a ...any) { m.errors = append(m.errors, fmt.Sprintf(f, a...)) }
func (m *tbMock) Fatalf(f string, a ...any) { m.fatals = append(m.fatals, fmt.Sprintf(f, a...)) }

func (m *tbMock) ok() bool { return len(m.errors) == 0 && len(m.fatals) == 0 }

// subjectHealthLine is a synthetic subject-side log line for the /health
// record: Tier-S fields are "-" (the reference empirical line has "0" for
// BYTES_RECEIVED and a UUID for X-REQUEST-ID, which are Tier-S fields
// emitted by real Envoy but not by envoy-go per Decision A).
const subjectHealthLine = `[2026-04-30T09:10:30.856Z] "GET /health HTTP/1.1" 200 - - 3 0 - "-" "Go-http-client/1.1" "-" "127.0.0.1:15006" "-"`

func TestAssertRecord_TierE_Match(t *testing.T) {
	// Subject line: Tier-S fields are "-" (envoy-go emits "-" per Decision A).
	// Reference line: Tier-S fields may have real values (curl/Envoy native).
	// AssertRecord should pass: Tier-S only checks subject == "-".
	subj, err := ParseLogLine(subjectHealthLine)
	if err != nil {
		t.Fatalf("parse subject: %v", err)
	}
	// Use empirical line[0] as reference (has "0" for BYTES_RECEIVED, UUID for X-REQUEST-ID).
	ref, err := ParseLogLine(empiricalLines[0])
	if err != nil {
		t.Fatalf("parse ref: %v", err)
	}
	// Normalize to make Tier-E fields match (USER-AGENT differs: curl vs Go-http-client).
	ref.UserAgent = subj.UserAgent
	m := &tbMock{}
	AssertRecord(m, 1, subj, ref)
	if !m.ok() {
		t.Errorf("unexpected failures: errors=%v fatals=%v", m.errors, m.fatals)
	}
}

func TestAssertRecord_TierE_Mismatch(t *testing.T) {
	subj, _ := ParseLogLine(empiricalLines[0])
	ref, _ := ParseLogLine(empiricalLines[0])
	subj.ResponseCode = "500" // force Tier-E mismatch
	m := &tbMock{}
	AssertRecord(m, 1, subj, ref)
	if m.ok() {
		t.Error("expected failure for RESPONSE_CODE mismatch")
	}
}

func TestAssertRecord_TierS_Violation(t *testing.T) {
	subj, _ := ParseLogLine(empiricalLines[0])
	subj.ResponseFlags = "UH" // subject must emit "-"
	m := &tbMock{}
	AssertRecord(m, 1, subj, subj)
	if m.ok() {
		t.Error("expected failure for RESPONSE_FLAGS Tier-S violation")
	}
}

func TestAssertRecord_TierF_StartTime_Bad(t *testing.T) {
	subj, _ := ParseLogLine(empiricalLines[0])
	subj.StartTime = "not-a-timestamp"
	ref, _ := ParseLogLine(empiricalLines[0])
	m := &tbMock{}
	AssertRecord(m, 1, subj, ref)
	if m.ok() {
		t.Error("expected failure for bad START_TIME format")
	}
}

func TestAssertRecord_TierF_Duration_Bad(t *testing.T) {
	subj, _ := ParseLogLine(empiricalLines[0])
	subj.Duration = "-1" // negative duration
	ref, _ := ParseLogLine(empiricalLines[0])
	m := &tbMock{}
	AssertRecord(m, 1, subj, ref)
	if m.ok() {
		t.Error("expected failure for negative DURATION")
	}
}

func TestAuthorityHost(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"127.0.0.1:15006", "127.0.0.1"},
		{"localhost:8080", "localhost"},
		{"127.0.0.1", "127.0.0.1"},
		{"", ""},
	}
	for _, tc := range cases {
		got := authorityHost(tc.in)
		if got != tc.want {
			t.Errorf("authorityHost(%q): got %q, want %q", tc.in, got, tc.want)
		}
	}
}
