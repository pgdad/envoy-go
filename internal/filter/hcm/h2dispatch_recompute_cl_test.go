package hcm

import (
	"strconv"
	"strings"
	"testing"

	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
)

// h2WireContentLength returns every content-length value the writer put on the
// response HEADERS block (w.headers[0]), in wire order. A slice, not a count,
// so a failure can name the offending values — and so the ABSENT case is
// distinguishable from a present "0" (reference_probe_discipline: empty is not
// zero).
func h2WireContentLength(t *testing.T, w *captureH2Writer) []string {
	t.Helper()
	if len(w.headers) == 0 {
		t.Fatalf("writer recorded no HEADERS frame")
	}
	var got []string
	for _, h := range w.headers[0] {
		if strings.EqualFold(h.Name, "content-length") {
			got = append(got, h.Value)
		}
	}
	return got
}

// TestWriteH2Reply_ContentLengthRecompute pins the ONE writeH2Reply behavior
// that phase 93's local-reply Content-Length rests on, and that NOTHING in the
// tree pinned before this test.
//
// The mechanism (h2dispatch.go writeH2Reply): the carrier is iterated as a
// slice and a field whose lowercased name is "content-length" has its value
// REPLACED by strconv.Itoa(len(body)). The field is never SYNTHESIZED — an
// absent content-length stays absent, unlike date/server which are appended as
// defaults.
//
// ⚠️ THE ABSENT ROW IS THE NEGATIVE CONTROL AND IT IS WHAT DISCRIMINATES.
// A rewrite-everything writer, a synthesize-always writer, and today's
// rewrite-if-present writer all agree on the two PRESENT rows. Only the absent
// row separates them, and only it can catch a change that starts stamping a
// content-length onto carriers that deliberately omit one.
//
// ⚠️ WHY THIS MATTERS TO THE ROUTER. Phase 93 makes h2LocalReplyHeaders emit a
// Content-Length carrying len(body). The wire value is correct under ANY
// composer value BECAUSE of the rewrite pinned here — so if this behavior
// ever changes, the router's composer becomes the sole source of wire truth
// and a wrong bodyLen would ship. This test is the tripwire for that.
//
// Errorf, not Fatalf, on every property: a row must report the arity fault and
// the value fault in the same run.
func TestWriteH2Reply_ContentLengthRecompute(t *testing.T) {
	const body = "bad gateway\n" // 12 bytes — the router's bad502Body shape.

	cases := []struct {
		name string
		// carrier is the pre-write header set handed to writeH2Reply.
		carrier filter_http.OrderedHeaders
		body    []byte
		// wantArity is how many content-length fields must reach the wire.
		wantArity int
		// wantValue is checked only when wantArity == 1.
		wantValue string
	}{
		{
			// The load-bearing row for phase 93: a PRESENT but WRONG value is
			// corrected from len(body). "999" cannot be produced by any
			// len(body) in this table, so a writer that merely echoed the
			// carrier would be caught here.
			name: "present_wrong_value_is_recomputed",
			carrier: filter_http.OrderedHeaders{
				{Name: "Content-Type", Value: "text/plain"},
				{Name: "Content-Length", Value: "999"},
			},
			body:      []byte(body),
			wantArity: 1,
			wantValue: "12",
		},
		{
			// ⚠️ NEGATIVE CONTROL. A pristine carrier must come out with the
			// field ABSENT — arity 0, not "0". writeH2Reply appends date and
			// server defaults but must NOT append a content-length.
			name: "absent_stays_absent",
			carrier: filter_http.OrderedHeaders{
				{Name: "Content-Type", Value: "text/plain"},
			},
			body:      []byte(body),
			wantArity: 0,
		},
		{
			// A present-and-correct value survives the rewrite unchanged.
			name: "present_correct_value_stays_correct",
			carrier: filter_http.OrderedHeaders{
				{Name: "Content-Type", Value: "text/plain"},
				{Name: "Content-Length", Value: strconv.Itoa(len(body))},
			},
			body:      []byte(body),
			wantArity: 1,
			wantValue: "12",
		},
		{
			// Bodyless: the recompute must read len(body), not the carrier.
			// Discriminates a writer that only rewrites when len(body) > 0.
			name: "present_wrong_value_bodyless_recomputes_to_zero",
			carrier: filter_http.OrderedHeaders{
				{Name: "Content-Type", Value: "text/plain"},
				{Name: "Content-Length", Value: "999"},
			},
			body:      nil,
			wantArity: 1,
			wantValue: "0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Snapshot the carrier BEFORE the call. writeH2Reply builds its
			// own hf slice and must never mutate resp.Headers — SPEC 93 4.1's
			// access-log / encode-chain argument rests on that separation, so
			// it is pinned here rather than assumed.
			before := make([]string, len(tc.carrier))
			for i, h := range tc.carrier {
				before[i] = h.Name + ":" + h.Value
			}

			w := &captureH2Writer{}
			if err := writeH2Reply(w, 502, tc.carrier, tc.body, nil); err != nil {
				t.Fatalf("writeH2Reply: %v", err)
			}

			got := h2WireContentLength(t, w)
			if len(got) != tc.wantArity {
				t.Errorf("wire content-length arity = %d %v, want %d", len(got), got, tc.wantArity)
			} else if tc.wantArity == 1 && got[0] != tc.wantValue {
				t.Errorf("wire content-length = %q, want %q (= len(body) = %d)", got[0], tc.wantValue, len(tc.body))
			}

			for i, h := range tc.carrier {
				if now := h.Name + ":" + h.Value; now != before[i] {
					t.Errorf("carrier field %d mutated by writeH2Reply: %q, was %q", i, now, before[i])
				}
			}
		})
	}
}
