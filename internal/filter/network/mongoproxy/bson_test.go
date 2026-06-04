package mongoproxy

import (
	"encoding/binary"
	"testing"
)

// le helpers build little-endian wire bytes for tests (mirrored by the 0049
// driver's builders at Task 14).
func leI32(v int32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(v))
	return b
}
func leI64(v int64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(v))
	return b
}

// doc builds a BSON document from raw element bytes (type+cstring-name+value...).
func doc(elems ...byte) []byte {
	body := append(elems, 0x00) // terminator
	total := int32(4 + len(body))
	return append(leI32(total), body...)
}

func cstr(s string) []byte { return append([]byte(s), 0x00) }

func TestBSON_Int32Element(t *testing.T) {
	// {"a": int32(7)}
	raw := doc(append(append([]byte{0x10}, cstr("a")...), leI32(7)...)...)
	d, err := parseBSON(raw)
	if err != nil {
		t.Fatalf("parseBSON: %v", err)
	}
	e, ok := d.first()
	if !ok || e.name != "a" || e.typ != 0x10 {
		t.Fatalf("first elem = %+v ok=%v", e, ok)
	}
	if e.val.(int32) != 7 {
		t.Errorf("val = %v, want 7", e.val)
	}
}

func TestBSON_ScalarTypes(t *testing.T) {
	// {"d": double, "b": bool, "n": null, "i64": int64}
	var elems []byte
	elems = append(elems, 0x01)
	elems = append(elems, cstr("d")...)
	dbl := make([]byte, 8)
	binary.LittleEndian.PutUint64(dbl, 0x3FF0000000000000) // 1.0
	elems = append(elems, dbl...)
	elems = append(elems, 0x08)
	elems = append(elems, cstr("b")...)
	elems = append(elems, 0x01) // true
	elems = append(elems, 0x0A)
	elems = append(elems, cstr("n")...)
	elems = append(elems, 0x12)
	elems = append(elems, cstr("i64")...)
	elems = append(elems, leI64(9000000000)...)
	d, err := parseBSON(doc(elems...))
	if err != nil {
		t.Fatalf("parseBSON: %v", err)
	}
	if len(d.elems) != 4 {
		t.Fatalf("got %d elems, want 4", len(d.elems))
	}
}

func TestBSON_UnknownTypeThrows(t *testing.T) {
	// 0x13 Decimal128 is NOT in the 14-type subset → error (upstream throw parity).
	raw := doc(append(append([]byte{0x13}, cstr("x")...), make([]byte, 16)...)...)
	if _, err := parseBSON(raw); err == nil {
		t.Fatalf("parseBSON accepted 0x13 Decimal128; want error")
	}
}

func TestBSON_UndefinedAndJSCodeThrow(t *testing.T) {
	for _, bad := range []byte{0x06, 0x0D} { // Undefined, JS code
		raw := doc(append([]byte{bad}, cstr("x")...)...)
		if _, err := parseBSON(raw); err == nil {
			t.Errorf("parseBSON accepted type 0x%02x; want error", bad)
		}
	}
}

func TestBSON_TruncatedUnderflow(t *testing.T) {
	// Declared docLength longer than the actual buffer → error.
	raw := doc(append(append([]byte{0x10}, cstr("a")...), leI32(7)...)...)
	if _, err := parseBSON(raw[:len(raw)-2]); err == nil {
		t.Fatalf("parseBSON accepted a truncated document; want error")
	}
}

func bstr(s string) []byte { // BSON string: int32 len (incl trailing NUL) + bytes + NUL
	out := leI32(int32(len(s) + 1))
	out = append(out, []byte(s)...)
	return append(out, 0x00)
}

func TestBSON_StringElement(t *testing.T) {
	raw := doc(append(append([]byte{0x02}, cstr("s")...), bstr("hello")...)...)
	d, err := parseBSON(raw)
	if err != nil {
		t.Fatalf("parseBSON: %v", err)
	}
	e, _ := d.find("s")
	if e.val.(string) != "hello" {
		t.Errorf("val = %q, want hello", e.val)
	}
}

func TestBSON_NestedDocument(t *testing.T) {
	// {"_id": {"x": int32(1)}} — a Document-typed _id (the MultiGet shape).
	inner := doc(append(append([]byte{0x10}, cstr("x")...), leI32(1)...)...)
	raw := doc(append(append([]byte{0x03}, cstr("_id")...), inner...)...)
	d, err := parseBSON(raw)
	if err != nil {
		t.Fatalf("parseBSON: %v", err)
	}
	e, ok := d.find("_id")
	if !ok || e.typ != 0x03 {
		t.Fatalf("_id elem = %+v ok=%v", e, ok)
	}
	if _, isDoc := e.val.(bsonDoc); !isDoc {
		t.Errorf("nested _id val type = %T, want bsonDoc", e.val)
	}
}

func TestBSON_ObjectIdAndBinaryAndRegex(t *testing.T) {
	var elems []byte
	elems = append(elems, 0x07) // ObjectId (12 bytes)
	elems = append(elems, cstr("oid")...)
	elems = append(elems, make([]byte, 12)...)
	elems = append(elems, 0x05) // Binary: int32 len + subtype + bytes
	elems = append(elems, cstr("bin")...)
	elems = append(elems, leI32(3)...)
	elems = append(elems, 0x00) // subtype
	elems = append(elems, []byte{1, 2, 3}...)
	elems = append(elems, 0x0B) // Regex: 2 cstrings
	elems = append(elems, cstr("re")...)
	elems = append(elems, cstr("^a")...)
	elems = append(elems, cstr("i")...)
	d, err := parseBSON(doc(elems...))
	if err != nil {
		t.Fatalf("parseBSON: %v", err)
	}
	if len(d.elems) != 3 {
		t.Fatalf("got %d elems, want 3", len(d.elems))
	}
}

func TestBSON_AsInt64(t *testing.T) {
	for _, tc := range []struct {
		v    any
		want int64
		ok   bool
	}{{int32(5), 5, true}, {int64(9), 9, true}, {float64(3.0), 3, true}, {"x", 0, false}} {
		got, ok := asInt64(tc.v)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("asInt64(%v) = %d,%v want %d,%v", tc.v, got, ok, tc.want, tc.ok)
		}
	}
}
