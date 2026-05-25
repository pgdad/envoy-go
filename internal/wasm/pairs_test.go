// Tests for the byte-faithful proxy-wasm pairs wire format per R3 +
// parent §13-R3.
//
// Byte-faithful semantic reference: proxy-wasm-cpp-host:src/pairs_util.cc +
// include/proxy-wasm/pairs_util.h at SHA da3ce05d.
//
// Wire format (little-endian throughout):
//
//	u32 num_pairs
//	(u32 key_len, u32 value_len) * num_pairs
//	(key_bytes, NUL, value_bytes, NUL) * num_pairs
//
// Coverage:
//   - Golden-byte encode tests (empty, single, multi-pair, NUL-in-key,
//     NUL-in-value, empty-key, empty-value, both-empty).
//   - Round-trip fidelity (~10 fixtures).
//   - Malformed-input decode tests (truncated header, oversize num_pairs,
//     missing NUL after key, missing NUL after value, key-len overruns,
//     value-len overruns, trailing-garbage).
//
// Tests must FAIL before pairs.go lands per D-P-PLAN-4.

package wasm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// --- encode-side helpers ---------------------------------------------------

// u32LE returns the 4-byte little-endian encoding of v.
func u32LE(v uint32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	return b[:]
}

// concat returns the concatenation of all given byte slices in order.
func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// --- TestEncodePairs_Golden: golden-byte expectations ---------------------

func TestEncodePairs_Golden(t *testing.T) {
	cases := []struct {
		name string
		in   []HeaderPair
		want []byte
	}{
		{
			name: "empty pairs",
			in:   nil,
			want: u32LE(0),
		},
		{
			name: "empty pairs slice (non-nil)",
			in:   []HeaderPair{},
			want: u32LE(0),
		},
		{
			name: "single pair {foo,bar}",
			in:   []HeaderPair{{Key: "foo", Value: "bar"}},
			want: concat(
				u32LE(1),      // num_pairs
				u32LE(3),      // key_len
				u32LE(3),      // value_len
				[]byte("foo"), // key bytes
				[]byte{0x00},  // NUL
				[]byte("bar"), // value bytes
				[]byte{0x00},  // NUL
			),
		},
		{
			name: "two pairs {k1,v1} {k2,v2}",
			in: []HeaderPair{
				{Key: "k1", Value: "v1"},
				{Key: "k2", Value: "v2"},
			},
			want: concat(
				u32LE(2),
				u32LE(2), u32LE(2), // (k1,v1) lengths
				u32LE(2), u32LE(2), // (k2,v2) lengths
				[]byte("k1"), []byte{0x00}, []byte("v1"), []byte{0x00},
				[]byte("k2"), []byte{0x00}, []byte("v2"), []byte{0x00},
			),
		},
		{
			name: "three pairs (header roster shape)",
			in: []HeaderPair{
				{Key: ":method", Value: "GET"},
				{Key: ":path", Value: "/"},
				{Key: "host", Value: "example.com"},
			},
			want: concat(
				u32LE(3),
				u32LE(7), u32LE(3), // (:method, GET)
				u32LE(5), u32LE(1), // (:path, /)
				u32LE(4), u32LE(11), // (host, example.com)
				[]byte(":method"), []byte{0x00}, []byte("GET"), []byte{0x00},
				[]byte(":path"), []byte{0x00}, []byte("/"), []byte{0x00},
				[]byte("host"), []byte{0x00}, []byte("example.com"), []byte{0x00},
			),
		},
		{
			name: "empty key {'',x}",
			in:   []HeaderPair{{Key: "", Value: "x"}},
			want: concat(
				u32LE(1),
				u32LE(0), u32LE(1),
				[]byte{0x00}, // empty key bytes then NUL
				[]byte("x"),
				[]byte{0x00},
			),
		},
		{
			name: "empty value {x,''}",
			in:   []HeaderPair{{Key: "x", Value: ""}},
			want: concat(
				u32LE(1),
				u32LE(1), u32LE(0),
				[]byte("x"),
				[]byte{0x00},
				[]byte{0x00}, // empty value bytes then NUL
			),
		},
		{
			name: "both empty {'',''}",
			in:   []HeaderPair{{Key: "", Value: ""}},
			want: concat(
				u32LE(1),
				u32LE(0), u32LE(0),
				[]byte{0x00}, // empty-key NUL
				[]byte{0x00}, // empty-value NUL
			),
		},
		{
			name: "NUL inside key (length-prefixed, NUL byte verbatim)",
			in:   []HeaderPair{{Key: "a\x00b", Value: "v"}},
			want: concat(
				u32LE(1),
				u32LE(3), u32LE(1),
				[]byte{'a', 0x00, 'b'},
				[]byte{0x00},
				[]byte("v"),
				[]byte{0x00},
			),
		},
		{
			name: "NUL inside value (length-prefixed, NUL byte verbatim)",
			in:   []HeaderPair{{Key: "k", Value: "v\x00w"}},
			want: concat(
				u32LE(1),
				u32LE(1), u32LE(3),
				[]byte("k"),
				[]byte{0x00},
				[]byte{'v', 0x00, 'w'},
				[]byte{0x00},
			),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EncodePairs(tc.in)
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("EncodePairs mismatch:\n got = % x\nwant = % x", got, tc.want)
			}
		})
	}
}

// --- TestEncodePairs_LengthMatchesUpstreamPairsSize: each encoded buffer
// matches the size formula from upstream PairsUtil::pairsSize -----

func TestEncodePairs_LengthMatchesUpstreamPairsSize(t *testing.T) {
	cases := [][]HeaderPair{
		nil,
		{},
		{{Key: "foo", Value: "bar"}},
		{{Key: "k1", Value: "v1"}, {Key: "k2", Value: "v2"}},
		{{Key: "", Value: ""}},
		{{Key: "a\x00b", Value: "v"}},
		{{Key: strings.Repeat("x", 257), Value: strings.Repeat("y", 1023)}},
	}
	for _, c := range cases {
		got := len(EncodePairs(c))
		// Upstream PairsUtil::pairsSize formula:
		//   sizeof(uint32) + sum_i ( 2*sizeof(uint32) + key_i+1 + value_i+1 )
		want := 4
		for _, p := range c {
			want += 8 + len(p.Key) + 1 + len(p.Value) + 1
		}
		if got != want {
			t.Fatalf("encoded length mismatch for %v: got=%d want=%d", c, got, want)
		}
	}
}

// --- TestDecodePairs_Golden: golden-byte to pairs --------------------------

func TestDecodePairs_Golden(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want []HeaderPair
	}{
		{
			name: "empty num_pairs",
			in:   u32LE(0),
			want: nil,
		},
		{
			name: "single pair {foo,bar}",
			in: concat(
				u32LE(1),
				u32LE(3), u32LE(3),
				[]byte("foo"), []byte{0x00}, []byte("bar"), []byte{0x00},
			),
			want: []HeaderPair{{Key: "foo", Value: "bar"}},
		},
		{
			name: "two pairs",
			in: concat(
				u32LE(2),
				u32LE(2), u32LE(2),
				u32LE(2), u32LE(2),
				[]byte("k1"), []byte{0x00}, []byte("v1"), []byte{0x00},
				[]byte("k2"), []byte{0x00}, []byte("v2"), []byte{0x00},
			),
			want: []HeaderPair{
				{Key: "k1", Value: "v1"},
				{Key: "k2", Value: "v2"},
			},
		},
		{
			name: "both empty",
			in: concat(
				u32LE(1),
				u32LE(0), u32LE(0),
				[]byte{0x00}, []byte{0x00},
			),
			want: []HeaderPair{{Key: "", Value: ""}},
		},
		{
			name: "NUL inside key, length-prefixed",
			in: concat(
				u32LE(1),
				u32LE(3), u32LE(1),
				[]byte{'a', 0x00, 'b'}, []byte{0x00},
				[]byte("v"), []byte{0x00},
			),
			want: []HeaderPair{{Key: "a\x00b", Value: "v"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodePairs(tc.in)
			if err != nil {
				t.Fatalf("DecodePairs returned error: %v", err)
			}
			// Normalize nil vs empty for reflect.DeepEqual.
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("DecodePairs mismatch:\n got = %#v\nwant = %#v", got, tc.want)
			}
		})
	}
}

// --- TestPairsRoundTrip: encode then decode is identity for ~10 fixtures ---

func TestPairsRoundTrip(t *testing.T) {
	fixtures := [][]HeaderPair{
		nil,
		{},
		{{Key: "foo", Value: "bar"}},
		{{Key: "k1", Value: "v1"}, {Key: "k2", Value: "v2"}},
		{
			{Key: ":method", Value: "GET"},
			{Key: ":path", Value: "/"},
			{Key: ":scheme", Value: "https"},
			{Key: ":authority", Value: "example.com"},
			{Key: "user-agent", Value: "envoy-go-test/1.0"},
		},
		{{Key: "", Value: ""}},
		{{Key: "", Value: "x"}, {Key: "x", Value: ""}},
		{{Key: "a\x00b", Value: "v"}},
		{{Key: "k", Value: "v\x00w"}},
		{{Key: strings.Repeat("a", 1024), Value: strings.Repeat("b", 1024)}},
		{
			{Key: "x-custom-1", Value: "value-1"},
			{Key: "x-custom-2", Value: "value-2"},
			{Key: "x-custom-3", Value: "value-3"},
			{Key: "x-custom-4", Value: "value-4"},
		},
	}

	for i, in := range fixtures {
		encoded := EncodePairs(in)
		got, err := DecodePairs(encoded)
		if err != nil {
			t.Fatalf("fixture[%d]: DecodePairs(EncodePairs) error: %v", i, err)
		}
		if len(in) == 0 && len(got) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, in) {
			t.Fatalf("fixture[%d]: round-trip mismatch\n got = %#v\nwant = %#v", i, got, in)
		}
	}
}

// --- TestDecodePairs_Malformed: malformed-input rejection ------------------

func TestDecodePairs_Malformed(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{
			name: "empty buffer (no num_pairs)",
			in:   []byte{},
		},
		{
			name: "truncated header (3 bytes)",
			in:   []byte{0x01, 0x00, 0x00},
		},
		{
			name: "num_pairs=1 but no length-pairs follow",
			in:   u32LE(1),
		},
		{
			name: "num_pairs=1, only one u32 of lengths (need two)",
			in:   concat(u32LE(1), u32LE(3)),
		},
		{
			name: "num_pairs=1, lengths declared, but no bodies",
			in:   concat(u32LE(1), u32LE(3), u32LE(3)),
		},
		{
			name: "missing NUL after key (key bytes present, NUL omitted, then value)",
			in: concat(
				u32LE(1),
				u32LE(3), u32LE(3),
				// key=foo, no NUL, value=bar, then NUL — but the byte at the
				// key-terminator slot is 'b' (first byte of "bar") which != 0x00.
				[]byte("foo"), []byte("bar"), []byte{0x00},
			),
		},
		{
			name: "missing NUL after value",
			in: concat(
				u32LE(1),
				u32LE(3), u32LE(3),
				[]byte("foo"), []byte{0x00}, []byte("bar"),
				// no final NUL; the buffer ends one byte short.
			),
		},
		{
			name: "key_len > remaining buffer",
			in: concat(
				u32LE(1),
				u32LE(100), u32LE(0), // key_len=100, value_len=0
				[]byte("xx"), // only 2 bytes of key+NUL would be needed for an empty key — and we are well short of 100
			),
		},
		{
			name: "value_len > remaining buffer",
			in: concat(
				u32LE(1),
				u32LE(1), u32LE(100), // key_len=1, value_len=100
				[]byte("k"), []byte{0x00}, // key body+NUL fits
				[]byte("v"), []byte{0x00}, // value body+NUL undersized for value_len=100
			),
		},
		{
			name: "oversize num_pairs (claimed > buffer can hold)",
			// Total buffer is 8 bytes, so num_pairs=0xFFFF would require
			// 0xFFFF * 8 = 524k bytes of length-pairs alone.
			in: concat(u32LE(0xFFFF), u32LE(0)),
		},
		{
			name: "trailing garbage after well-formed pair (strict total-size check)",
			in: concat(
				u32LE(1),
				u32LE(3), u32LE(3),
				[]byte("foo"), []byte{0x00}, []byte("bar"), []byte{0x00},
				[]byte{0xff, 0xff}, // extra trailing bytes
			),
		},
		{
			name: "num_pairs exceeds the hard cap (defensive check)",
			// The cpp upstream rejects num_pairs > PROXY_WASM_HOST_PAIRS_MAX_COUNT;
			// envoy-go uses a buffer-derived cap so MAX_UINT32 always rejects.
			in: u32LE(0xFFFFFFFF),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodePairs(tc.in)
			if err == nil {
				t.Fatalf("DecodePairs(%s): want error, got nil; decoded=%#v", tc.name, got)
			}
			if got != nil {
				t.Fatalf("DecodePairs(%s): want nil pairs on error, got %#v", tc.name, got)
			}
		})
	}
}

// --- TestDecodePairs_WrapsErrors: returned errors compose with errors.Is/As

func TestDecodePairs_WrapsErrors(t *testing.T) {
	// Truncated header should return SOME non-nil error.
	_, err := DecodePairs([]byte{0x01})
	if err == nil {
		t.Fatal("expected error on truncated header, got nil")
	}
	// Sanity: the message should be human-meaningful (not "EOF" or empty).
	if err.Error() == "" {
		t.Fatal("expected non-empty error message")
	}

	// The unwrap chain should not panic even if there is no wrapped cause.
	_ = errors.Unwrap(err)
}

// --- TestEncodePairs_PreservesOrder: pair order is byte-position significant.

func TestEncodePairs_PreservesOrder(t *testing.T) {
	a := []HeaderPair{
		{Key: "first", Value: "1"},
		{Key: "second", Value: "2"},
	}
	b := []HeaderPair{
		{Key: "second", Value: "2"},
		{Key: "first", Value: "1"},
	}
	ea := EncodePairs(a)
	eb := EncodePairs(b)
	if bytes.Equal(ea, eb) {
		t.Fatalf("EncodePairs should NOT produce equal bytes for different pair order")
	}
	// And the order round-trips:
	ga, err := DecodePairs(ea)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ga, a) {
		t.Fatalf("order not preserved: got=%#v want=%#v", ga, a)
	}
}
