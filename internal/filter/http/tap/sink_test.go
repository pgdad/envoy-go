package tap

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	datatapv3 "github.com/envoyproxy/go-control-plane/envoy/data/tap/v3"
	"google.golang.org/protobuf/encoding/protojson"
)

// canonJSON removes protojson's build-seeded detrand whitespace (internal/detrand
// randomizes spaces/indent per binary, so raw bytes differ between `go test` and
// `-race`). Compacting to canonical JSON keeps the token stream — keys, values,
// ordering, and the presence/absence of fields like "body" — while erasing only
// insignificant whitespace, so the comparison is build-independent yet still
// catches an EmitUnpopulated regression ("body":null is a real token) and any
// field-name/structure/ordering drift.
func canonJSON(t *testing.T, b []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		t.Fatalf("json.Compact: %v (not valid JSON): %s", err, b)
	}
	return buf.String()
}

func goldenTrace() *datatapv3.TraceWrapper {
	return &datatapv3.TraceWrapper{Trace: &datatapv3.TraceWrapper_HttpBufferedTrace{
		HttpBufferedTrace: &datatapv3.HttpBufferedTrace{
			Request:  &datatapv3.HttpBufferedTrace_Message{Headers: []*corev3.HeaderValue{{Key: ":method", Value: "GET"}}},
			Response: &datatapv3.HttpBufferedTrace_Message{Headers: []*corev3.HeaderValue{{Key: ":status", Value: "204"}}},
		},
	}}
}

// The exact wire shape the reference produces. If protojson's output ever
// drifts (a toolchain change, protojson's detrand), THIS test must fail loudly
// rather than be "fixed" by regenerating the golden.
func TestMarshal_ByteExactGolden(t *testing.T) {
	got, err := marshalOpts.Marshal(goldenTrace())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{
 "http_buffered_trace": {
  "request": {
   "headers": [
    {
     "key": ":method",
     "value": "GET",
     "raw_value": ""
    }
   ],
   "trailers": []
  },
  "response": {
   "headers": [
    {
     "key": ":status",
     "value": "204",
     "raw_value": ""
    }
   ],
   "trailers": []
  }
 }
}`
	if canonJSON(t, got) != canonJSON(t, []byte(want)) {
		t.Errorf("protojson structure drift.\n got:\n%s\nwant:\n%s", got, want)
	}
	// Positive pins on the two properties the differential depends on.
	if bytes.Contains(got, []byte(`"body"`)) {
		t.Errorf(`"body" must be ABSENT (nil message field omitted by EmitDefaultValues)`)
	}
	gotCanon := canonJSON(t, got)
	if !strings.Contains(gotCanon, `"trailers":[]`) {
		t.Errorf(`"trailers": [] must be present (nil repeated rendered by EmitDefaultValues)`)
	}
	if !strings.Contains(gotCanon, `"raw_value":""`) {
		t.Errorf(`"raw_value": "" must be present`)
	}
}

// EmitUnpopulated is the wrong option and must be provably different.
func TestMarshal_EmitUnpopulatedWouldEmitNullBody(t *testing.T) {
	wrong := protojson.MarshalOptions{Multiline: true, Indent: " ", UseProtoNames: true, EmitUnpopulated: true}
	b, err := wrong.Marshal(goldenTrace())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(canonJSON(t, b), `"body":null`) {
		t.Fatalf("expected EmitUnpopulated to emit \"body\": null; the option semantics changed, re-derive AMEND-TAP-JSON")
	}
}

func TestMarshal_StableAcrossRepeatedCalls(t *testing.T) {
	first, err := marshalOpts.Marshal(goldenTrace())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for i := 0; i < 100; i++ {
		b, err := marshalOpts.Marshal(goldenTrace())
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if !bytes.Equal(b, first) {
			t.Fatalf("protojson output is not byte-stable within a process (iteration %d)", i)
		}
	}
}

func TestSink_WritesOneFilePerTrace_WithTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	s, err := newFilePerTapSink(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatalf("newFilePerTapSink: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := s.write(goldenTrace()); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "out_*.json"))
	if len(files) != 3 {
		t.Fatalf("files = %d, want 3 (one DISCRETE file per trace)", len(files))
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if !bytes.HasSuffix(b, []byte("\n")) {
			t.Errorf("%s: must end with a trailing newline", filepath.Base(f))
		}
		if !strings.HasPrefix(filepath.Base(f), "out_") || !strings.HasSuffix(f, ".json") {
			t.Errorf("%s: want <prefix>_<trace_id>.json", filepath.Base(f))
		}
	}
}

// D-TAP-PATHPREFIX: the parent directory is created at sink construction.
func TestSink_MkdirAllsParentAtConstruction(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deeper")
	if _, err := newFilePerTapSink(filepath.Join(dir, "out")); err != nil {
		t.Fatalf("newFilePerTapSink must MkdirAll the parent: %v", err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("parent dir %s was not created", dir)
	}
}

func TestSink_RejectsUncreatableParent(t *testing.T) {
	f := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// parent of <file>/sub/out is <file>/sub — cannot be created under a regular file
	if _, err := newFilePerTapSink(filepath.Join(f, "sub", "out")); err == nil {
		t.Errorf("expected a parse-time reject for an uncreatable path_prefix parent")
	}
}

// marshalBody renders b through a minimal HttpBufferedTrace and canonicalizes
// the JSON (reusing canonJSON — no re-implementation of json.Compact).
func marshalBody(t *testing.T, b *datatapv3.Body) string {
	t.Helper()
	raw, err := marshalOpts.Marshal(&datatapv3.HttpBufferedTrace{
		Request: &datatapv3.HttpBufferedTrace_Message{Body: b},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err) // AS_STRING sanitize must PREVENT this
	}
	return canonJSON(t, raw)
}

func TestBodyProto_AsString(t *testing.T) {
	s := marshalBody(t, bodyProto([]byte("0123456789"), false, true))
	if !strings.Contains(s, `"as_string":"0123456789"`) {
		t.Errorf("AS_STRING render missing as_string: %s", s)
	}
	if strings.Contains(s, `"as_bytes"`) {
		t.Errorf("AS_STRING must NOT render as_bytes: %s", s)
	}
	if !strings.Contains(s, `"truncated":false`) {
		t.Errorf("truncated:false must be emitted (EmitDefaultValues): %s", s)
	}
}

func TestBodyProto_AsBytesBase64(t *testing.T) {
	// AMEND-TAP-BODY2-BYTES: "abcdefghij" -> "YWJjZGVmZ2hpag==" (Go-native).
	s := marshalBody(t, bodyProto([]byte("abcdefghij"), false, false))
	if !strings.Contains(s, `"as_bytes":"YWJjZGVmZ2hpag=="`) {
		t.Errorf("AS_BYTES render wrong base64: %s", s)
	}
	if strings.Contains(s, `"as_string"`) {
		t.Errorf("AS_BYTES must NOT render as_string: %s", s)
	}
}

func TestBodyProto_TruncatedTrue(t *testing.T) {
	s := marshalBody(t, bodyProto([]byte("0123456789"), true, true))
	if !strings.Contains(s, `"truncated":true`) {
		t.Errorf("truncated:true must render: %s", s)
	}
}

func TestBodyProto_AsStringSanitizesNonUTF8(t *testing.T) {
	// D-TAP-BODY-UTF8: raw string(buf) would make protojson.Marshal ERROR and
	// the sink swallow it -> whole-trace drop. Sanitize keeps the trace.
	nonUTF8 := []byte{0xff, 0xfe, 0x41, 0x42}            // invalid UTF-8 + "AB"
	s := marshalBody(t, bodyProto(nonUTF8, false, true)) // marshalBody Fatalfs on error
	if !strings.Contains(s, "AB") {
		t.Errorf("sanitized as_string should retain the valid tail: %s", s)
	}
}
