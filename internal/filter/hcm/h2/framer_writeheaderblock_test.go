package h2

import (
	"bytes"
	"io"
	"testing"

	"golang.org/x/net/http2"
)

// ---------------------------------------------------------------------------
// phase-88 (h2-continuation-frames): the writeHeaderBlock roster.
//
// ⚠️ THIS FILE DOES NOT COMPILE AGAINST THE UNFIXED TREE — that is the point.
// (*framer).writeHeaderBlock does not exist at the tip; the whole package fails
// to build until the phase-88 IMPL lands it. The behavioral CONTINUATION arms
// therefore live in a SEPARATE file (continuation_test.go) whose RED census was
// taken BEFORE this file was added, because a build failure here would mask
// every one of them.
//
// ⚠️ THE BINARY-ASSERTION TRAP (PLAN-88 §9.5): `strings <binary> | grep -c
// writeHeaderBlock` reads 9 on the UNMODIFIED tip — Go's bundled net/http has
// http2writeResHeaders.writeHeaderBlock and http2writePushPromise.
// writeHeaderBlock. Only the QUALIFIED name discriminates.
//
// The roster is 11 rows, not the 4 originally proposed, because a break arm
// that hardcodes 16384 and ignores the peer's advertised SETTINGS_MAX_FRAME_SIZE
// reddens NONE of those 4 (PLAN-88 §5.1). peer_max_larger and
// peer_max_max_legal are the rows that catch it.
// ---------------------------------------------------------------------------

// whbStreamID is deliberately NOT 1: every emitted frame must carry the
// caller's stream id, and a hardcoded 1 would pass unnoticed against it.
const whbStreamID = uint32(7)

// whbFill produces a POSITION-DEPENDENT byte pattern.
//
// ⚠️ THIS IS LOAD-BEARING, NOT DECORATION. A frame-COUNT assertion is
// compensating-defect-blind: two off-by-one slice errors that cancel produce
// the right number of frames carrying the wrong bytes. With a constant fill
// (all zeros, or a repeated pattern) a mis-sliced fragment ALIASES a correctly
// sliced one and byte-exact reassembly still passes. byte(i*31+7) has period
// 256 in i and never repeats within any window smaller than that, so any
// shifted or duplicated slice shows up in the reassembly compare.
func whbFill(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*31 + 7)
	}
	return b
}

// whbReadBack replays every frame written into buf.
//
// The reader's max frame size is raised to the RFC 9113 ceiling because the
// peer_max_larger / peer_max_max_legal rows legitimately emit frames well above
// the 16384 spec floor; a reader left at the default would report a
// FRAME_SIZE_ERROR and the row would fail for the wrong reason. AllowIllegalReads
// is deliberately NOT set: a correct writeHeaderBlock emits a LEGAL
// HEADERS+CONTINUATION* sequence, so x/net's own checkFrameOrder is a free extra
// assertion that the flag placement is well-formed.
//
// ⚠️ Fragments returned by ReadFrame alias an internal buffer that the next
// ReadFrame call invalidates, so each fragment is COPIED as it is read.
func whbReadBack(t *testing.T, buf *bytes.Buffer) ([]http2.Frame, [][]byte, error) {
	t.Helper()
	rd := http2.NewFramer(io.Discard, bytes.NewReader(buf.Bytes()))
	rd.SetMaxReadFrameSize(1<<24 - 1)
	var frames []http2.Frame
	var frags [][]byte
	for {
		f, err := rd.ReadFrame()
		if err == io.EOF {
			return frames, frags, nil
		}
		if err != nil {
			return frames, frags, err
		}
		var frag []byte
		switch tf := f.(type) {
		case *http2.HeadersFrame:
			frag = tf.HeaderBlockFragment()
		case *http2.ContinuationFrame:
			frag = tf.HeaderBlockFragment()
		}
		cp := make([]byte, len(frag))
		copy(cp, frag)
		frames = append(frames, f)
		frags = append(frags, cp)
	}
}

// TestFramer_WriteHeaderBlock_Table is the 11-row roster.
//
// Every row asserts SIX independent properties, because the framing can be
// wrong in six independent ways:
//
//  1. the frame COUNT,
//  2. the frame TYPES in order (HEADERS first, CONTINUATION after),
//  3. each frame's FRAGMENT LENGTH (the per-hop bound, and the boundary
//     arithmetic that a count alone cannot see),
//  4. END_HEADERS on the LAST emitted frame ONLY,
//  5. END_STREAM on the HEADERS frame only — RFC 9113 §6.10 gives CONTINUATION
//     no flag but END_HEADERS,
//  6. byte-exact reassembly, and every frame's StreamID.
//
// Assertions use t.Errorf, never t.Fatalf, so one wrong property inside a row
// does not make the other five unreachable dead code.
func TestFramer_WriteHeaderBlock_Table(t *testing.T) {
	tests := []struct {
		name string
		// peerMaxFrameSize as advertised by the peer; 0 means "peer has not
		// sent SETTINGS", which RFC 9113 §6.5.2 defaults to 16384.
		peerMax int32
		// blockLen is the HPACK-ENCODED block length. ⚠️ The control variable
		// is the ENCODED size, never a raw header byte count (PLAN-88 §1.1):
		// Huffman coding shrinks values 20-40% and entropy silently moves the
		// split point, so the roster feeds writeHeaderBlock an exact-length
		// block directly rather than encoding headers and hoping.
		blockLen  int
		endStream bool
		// padLength / priority exercise the FIRST frame's non-fragment payload
		// overhead. SETTINGS_MAX_FRAME_SIZE bounds the frame PAYLOAD, not the
		// fragment, and a HEADERS payload also carries the pad-length byte, the
		// padding itself, and the 5-byte PRIORITY block — so writeHeaderBlock
		// must subtract that overhead from the first frame's fragment budget.
		// Without these two rows the guard has NO coverage: deleting the
		// subtraction entirely leaves all other rows GREEN (verified).
		padLength uint8
		priority  http2.PriorityParam
		// wantFragLens is the expected per-frame fragment length, in order.
		// Spelled out as a literal per row on purpose: deriving it from the
		// same ceil() the implementation uses would make the row circular.
		wantFragLens []int
	}{
		// --- the four originally-proposed rows ---
		{
			name: "under_max", peerMax: 16384, blockLen: 100,
			wantFragLens: []int{100},
		},
		{
			// Off-by-one at the boundary: exactly maxFrame must NOT spill into
			// a CONTINUATION.
			name: "exactly_max", peerMax: 16384, blockLen: 16384,
			wantFragLens: []int{16384},
		},
		{
			name: "one_over_max", peerMax: 16384, blockLen: 16385,
			wantFragLens: []int{16384, 1},
		},
		{
			// PLAN-88 §5.3: this is NOT hypothetical. Neither ServerConn.clientS
			// nor ClientConn.serverS is seeded, and both assign MaxFrameSize only
			// if the peer's SETTINGS frame carries the parameter — which RFC 9113
			// §6.5 does not require. driveHandshake (settings_validate_test.go)
			// sends WriteSettings() with NO arguments, so an entire existing suite
			// already runs at MaxFrameSize == 0. The 16384 default must come from
			// the SIBLING guard already landed twice (conn.go writeData,
			// client.go writeData), not a newly minted policy.
			name: "zero_max_defaults_16384", peerMax: 0, blockLen: 16385,
			wantFragLens: []int{16384, 1},
		},

		// --- rows the 4-row framing misses ---
		{
			// ONE HEADERS frame with an empty fragment and END_HEADERS — never
			// zero frames. A "range over chunks" implementation emits nothing.
			name: "empty_block", peerMax: 16384, blockLen: 0,
			wantFragLens: []int{0},
		},
		{
			// No trailing EMPTY CONTINUATION. Nothing else in the roster
			// detects one.
			name: "exact_multiple_2x", peerMax: 16384, blockLen: 32768,
			wantFragLens: []int{16384, 16384},
		},
		{
			name: "exact_multiple_3x", peerMax: 16384, blockLen: 49152,
			wantFragLens: []int{16384, 16384, 16384},
		},
		{
			// Catches a non-looping "at most one CONTINUATION" implementation.
			name: "three_continuations", peerMax: 16384, blockLen: 49153,
			wantFragLens: []int{16384, 16384, 16384, 1},
		},
		{
			// ⚠️ THE HIGHEST-VALUE ROW: it is the one a break arm that hardcodes
			// 16384 and ignores the peer's advertised value reddens. Every row
			// above stays GREEN against that break.
			name: "peer_max_larger", peerMax: 65536, blockLen: 100000,
			wantFragLens: []int{65536, 34464},
		},
		{
			// Same discriminator at the RFC 9113 §6.5.2 ceiling (2^24-1): the
			// whole block must ride ONE frame.
			name: "peer_max_max_legal", peerMax: 16777215, blockLen: 40000,
			wantFragLens: []int{40000},
		},
		{
			// END_STREAM rides the HEADERS frame ONLY. RFC 9113 §6.10 defines
			// exactly one flag on CONTINUATION (END_HEADERS); leaking END_STREAM
			// onto the last CONTINUATION is an ISOLATING break — this row alone
			// reddens.
			name: "end_stream_with_continuation", peerMax: 16384, blockLen: 16385,
			endStream:    true,
			wantFragLens: []int{16384, 1},
		},
		// --- first-frame payload OVERHEAD rows (added at the phase-88 IMPL's
		// adversarial review, which proved the overhead guard had ZERO coverage
		// while the ADR asserted the roster protected it) ---
		{
			// PadLength adds 1 (the length byte) + 8 (the padding) to the
			// HEADERS payload, so the first fragment must shrink by 9 and the
			// remainder spills into a CONTINUATION that carries no such
			// overhead. Without the subtraction the HEADERS payload would be
			// 16384 + 9 = 16393 — illegal against the advertised 16384.
			name: "pad_length_overhead_shrinks_first_frame", peerMax: 16384, blockLen: 16384,
			padLength:    8,
			wantFragLens: []int{16375, 9},
		},
		{
			// The PRIORITY block adds exactly 5 payload bytes (4 stream-dep +
			// 1 weight) to the HEADERS frame only. RFC 9113 §6.10 gives a
			// CONTINUATION no PRIORITY, so only the first frame's budget moves.
			name: "priority_overhead_shrinks_first_frame", peerMax: 16384, blockLen: 16384,
			priority:     http2.PriorityParam{StreamDep: 3, Weight: 15},
			wantFragLens: []int{16379, 5},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			block := whbFill(tc.blockLen)
			var buf bytes.Buffer
			f := &framer{Framer: http2.NewFramer(&buf, nil)}

			if err := f.writeHeaderBlock(http2.HeadersFrameParam{
				StreamID:      whbStreamID,
				BlockFragment: block,
				EndStream:     tc.endStream,
				PadLength:     tc.padLength,
				Priority:      tc.priority,
				// ⚠️ DELIBERATELY WRONG. The caller's EndHeaders is IGNORED and
				// RECOMPUTED: passing true on a block that must split would, if
				// honored, produce a HEADERS(END_HEADERS) followed by an orphan
				// CONTINUATION — which whbReadBack's framer rejects outright with
				// "unexpected CONTINUATION", so this is falsifiable, not cosmetic.
				EndHeaders: true,
			}, tc.peerMax); err != nil {
				t.Fatalf("writeHeaderBlock: %v", err)
			}

			frames, frags, rerr := whbReadBack(t, &buf)
			if rerr != nil {
				t.Errorf("reading emitted frames back: %v (a correct writeHeaderBlock emits a LEGAL HEADERS+CONTINUATION* sequence)", rerr)
			}

			// (1) frame count.
			if len(frames) != len(tc.wantFragLens) {
				t.Errorf("frames = %d, want %d", len(frames), len(tc.wantFragLens))
			}

			for i, fr := range frames {
				// (2) frame types in order.
				if i == 0 {
					if _, ok := fr.(*http2.HeadersFrame); !ok {
						t.Errorf("frame 0 type = %T, want *http2.HeadersFrame", fr)
					}
				} else if _, ok := fr.(*http2.ContinuationFrame); !ok {
					t.Errorf("frame %d type = %T, want *http2.ContinuationFrame", i, fr)
				}

				// (6a) every emitted frame carries the caller's stream id.
				if got := fr.Header().StreamID; got != whbStreamID {
					t.Errorf("frame %d StreamID = %d, want %d", i, got, whbStreamID)
				}

				// (3) per-frame fragment length.
				if i < len(tc.wantFragLens) && len(frags[i]) != tc.wantFragLens[i] {
					t.Errorf("frame %d fragment len = %d, want %d", i, len(frags[i]), tc.wantFragLens[i])
				}
				if int(fr.Header().Length) > whbEffectiveMax(tc.peerMax) {
					t.Errorf("frame %d payload len = %d, exceeds the peer's effective max frame size %d",
						i, fr.Header().Length, whbEffectiveMax(tc.peerMax))
				}

				// (4) END_HEADERS on the LAST frame only.
				endHeaders := fr.Header().Flags.Has(http2.FlagHeadersEndHeaders)
				wantEndHeaders := i == len(frames)-1
				if endHeaders != wantEndHeaders {
					t.Errorf("frame %d END_HEADERS = %v, want %v (set on the last emitted frame ONLY)", i, endHeaders, wantEndHeaders)
				}

				// (5) END_STREAM on the HEADERS frame only.
				endStream := fr.Header().Flags.Has(http2.FlagHeadersEndStream)
				wantEndStream := i == 0 && tc.endStream
				if endStream != wantEndStream {
					t.Errorf("frame %d (%v) END_STREAM = %v, want %v; flags=0x%x",
						i, fr.Header().Type, endStream, wantEndStream, uint8(fr.Header().Flags))
				}
			}

			// (6b) byte-exact reassembly.
			var reassembled []byte
			for _, fg := range frags {
				reassembled = append(reassembled, fg...)
			}
			if !bytes.Equal(reassembled, block) {
				t.Errorf("reassembled block != input: got %d bytes, want %d bytes; first divergence at %d",
					len(reassembled), len(block), whbFirstDiff(reassembled, block))
			}
		})
	}
}

// whbEffectiveMax mirrors the RFC 9113 §6.5.2 default the roster requires:
// a non-positive advertised value means the peer has not sent SETTINGS.
func whbEffectiveMax(peerMax int32) int {
	if peerMax <= 0 {
		return 16384
	}
	return int(peerMax)
}

// whbFirstDiff returns the index of the first differing byte, or -1 when the
// two slices agree over their common prefix. It exists so a reassembly failure
// names WHERE the slicing went wrong instead of only that it did.
func whbFirstDiff(got, want []byte) int {
	n := len(got)
	if len(want) < n {
		n = len(want)
	}
	for i := 0; i < n; i++ {
		if got[i] != want[i] {
			return i
		}
	}
	if len(got) != len(want) {
		return n
	}
	return -1
}
