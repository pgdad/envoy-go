package driver

import (
	"net/http"
	"strings"
	"testing"
)

// TestEncodeProbe_ContinueShape pins the encodeProbe output for a typical
// continue-mode response: 200 + route-count header + "backend\n" body.
func TestEncodeProbe_ContinueShape(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Header: http.Header{
			"X-Envoy-Go-Test-Route-Count": []string{"7"},
			"Content-Type":                []string{"text/plain"},
		},
	}
	p := probe{mode: "continue"}
	out := encodeProbe(1, p, resp, []byte("backend\n"))

	for _, line := range []string{
		"=== request 1 mode=continue",
		"status: 200",
		"header: content-type: text/plain",
		"header: x-envoy-go-test-route-count: 7",
		`body: "backend\n"`,
	} {
		if !strings.Contains(out, line) {
			t.Errorf("expected line %q in output, got:\n%s", line, out)
		}
	}
}

// TestEncodeProbe_LocalReplyShape pins the local-reply-decode mode encoding:
// 418 + route-count header (still echoed by the encode chain after
// SendLocalReply) + teapot body.
func TestEncodeProbe_LocalReplyShape(t *testing.T) {
	resp := &http.Response{
		StatusCode: 418,
		Header: http.Header{
			"X-Envoy-Go-Test-Route-Count": []string{"7"},
			"Content-Type":                []string{"text/plain"},
		},
	}
	p := probe{mode: "local-reply-decode"}
	out := encodeProbe(4, p, resp, []byte("i am a teapot\n"))

	for _, line := range []string{
		"=== request 4 mode=local-reply-decode",
		"status: 418",
		"header: x-envoy-go-test-route-count: 7",
		`body: "i am a teapot\n"`,
	} {
		if !strings.Contains(out, line) {
			t.Errorf("expected line %q in output, got:\n%s", line, out)
		}
	}
}

// TestEncodeProbe_ModifyEncodeDataShape pins the modify-encode-data mode
// expected body shape: "MODIFIED" (8 bytes) — the result of copy("MODIFIED\n",
// "backend\n") which writes only the first 8 bytes due to slice length.
func TestEncodeProbe_ModifyEncodeDataShape(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Header: http.Header{
			"X-Envoy-Go-Test-Route-Count": []string{"7"},
		},
	}
	p := probe{mode: "modify-encode-data"}
	out := encodeProbe(7, p, resp, []byte("MODIFIED"))

	for _, line := range []string{
		"=== request 7 mode=modify-encode-data",
		"status: 200",
		`body: "MODIFIED"`,
	} {
		if !strings.Contains(out, line) {
			t.Errorf("expected line %q in output, got:\n%s", line, out)
		}
	}
}

// TestModeProbes_OrderMatchesExpectations pins the parallel ordering
// invariant between modeProbes and modeExpectations. A future reorder of one
// without the other surfaces here loudly rather than producing a silent
// per-mode cross-up that would be diagnosed only via integration failures.
func TestModeProbes_OrderMatchesExpectations(t *testing.T) {
	if len(modeProbes) != len(modeExpectations) {
		t.Fatalf("len(modeProbes)=%d != len(modeExpectations)=%d", len(modeProbes), len(modeExpectations))
	}
	for i, p := range modeProbes {
		if p.mode != modeExpectations[i].mode {
			t.Errorf("modeProbes[%d].mode=%q != modeExpectations[%d].mode=%q", i, p.mode, i, modeExpectations[i].mode)
		}
	}
}

// TestModeExpectations_AllEightCovered pins the 8 SPEC §7.3 modes are
// exhaustively enumerated. Adds defense against a future copy-paste error
// that drops a mode from the table.
func TestModeExpectations_AllEightCovered(t *testing.T) {
	want := map[string]bool{
		"continue":                false,
		"stop-and-resume-headers": false,
		"stop-and-buffer-data":    false,
		"local-reply-decode":      false,
		"local-reply-decode-data": false,
		"modify-encode-headers":   false,
		"modify-encode-data":      false,
		"stop-trailers":           false,
	}
	for _, e := range modeExpectations {
		if _, ok := want[e.mode]; !ok {
			t.Errorf("unknown mode in modeExpectations: %q", e.mode)
		}
		want[e.mode] = true
	}
	for m, seen := range want {
		if !seen {
			t.Errorf("mode %q missing from modeExpectations", m)
		}
	}
}

// TestDriver_RegisteredAtInit pins the init() registration so a future
// rename of fixtureName surfaces here.
func TestDriver_RegisteredAtInit(t *testing.T) {
	if fixtureName != "0007b-iteration-probe" {
		t.Errorf("fixtureName drift: got %q, want %q", fixtureName, "0007b-iteration-probe")
	}
}

// TestDriver_RequiresReferenceFalse pins the reference-less invariant. A
// future mistake that flips this to true would cause the runner to spawn
// reference Envoy for the iteration-probe fixture (which does not exist
// in upstream Envoy and would fail at typed_config parse).
func TestDriver_RequiresReferenceFalse(t *testing.T) {
	d := iterationProbeDriver{}
	if d.RequiresReference() {
		t.Errorf("RequiresReference() = true; want false (envoy-go-only structural fixture)")
	}
}
