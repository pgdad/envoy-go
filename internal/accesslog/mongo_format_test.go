package accesslog

import (
	"strings"
	"testing"
	"time"
)

func TestMongoFormatterGolden(t *testing.T) {
	rec := &MongoRecord{
		Time:         time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		Message:      `{"opcode": "OP_QUERY", "id": 1, "collection": "db.collection1"}`,
		UpstreamHost: "127.0.0.1:27017",
	}
	line := MongoFormat(rec)
	// The time field is asserted by SHAPE (RFC3339-ish), the rest by value.
	if !strings.HasPrefix(string(line), `{"time":"2026-06-06T12:00:00`) {
		t.Errorf("time field shape wrong: %q", line)
	}
	if !strings.Contains(string(line), `"upstream_host":"127.0.0.1:27017"`) {
		t.Errorf("upstream_host missing: %q", line)
	}
	if !strings.HasSuffix(string(line), "}\n") {
		t.Errorf("line must be one JSON object + newline: %q", line)
	}
}

func TestMongoFormatterUpstreamHostDash(t *testing.T) {
	rec := &MongoRecord{Time: time.Unix(0, 0).UTC(), Message: "{}", UpstreamHost: ""}
	if !strings.Contains(string(MongoFormat(rec)), `"upstream_host":"-"`) {
		t.Error("empty upstream host must render as \"-\" (Envoy missing-value convention)")
	}
}
