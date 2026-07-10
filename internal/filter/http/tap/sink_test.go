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
