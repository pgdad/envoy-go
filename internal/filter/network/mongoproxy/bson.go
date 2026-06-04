package mongoproxy

import (
	"encoding/binary"
	"errors"
	"math"
)

// errBSON wraps a BSON decode failure. The codec converts any BSON error into
// the decoding_error path (§3.5). Wording is internal-only (never byte-compared)
// — the differential proof is the decoding_error COUNT, not the message.
func errBSON(msg string) error { return errors.New("mongoproxy: bson: " + msg) }

// bsonElem is one document element in wire order. val holds the decoded Go value
// per type (Int32→int32, Int64/Datetime/Timestamp→int64, Double→float64,
// Boolean→bool, String/Symbol→string, ObjectId/Binary→[]byte, Document/Array→
// bsonDoc, Null→nil, Regex→[2]string). The codec reads only first()/find() +
// asInt64 (D-S29.1-2) — the full materialization mirrors upstream's eager parse.
type bsonElem struct {
	name string
	typ  byte
	val  any
}

// bsonDoc is a parsed BSON document (ordered elements). No exported API.
type bsonDoc struct {
	elems []bsonElem
}

func (d bsonDoc) first() (bsonElem, bool) {
	if len(d.elems) == 0 {
		return bsonElem{}, false
	}
	return d.elems[0], true
}

func (d bsonDoc) find(name string) (bsonElem, bool) {
	for _, e := range d.elems {
		if e.name == name {
			return e, true
		}
	}
	return bsonElem{}, false
}

// asInt64 unifies the numeric BSON types for the $maxTimeMS check (D-S29.1-2).
func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

// remaining returns the unread byte count (used by the codec to detect an
// optional trailing returnFieldsSelector after the OP_QUERY query document).
func (r *bsonReader) remaining() int { return len(r.buf) - r.pos }

// bsonReader is a little-endian cursor over a byte slice. Every read bounds-checks
// and returns an error on underflow (upstream throw parity).
type bsonReader struct {
	buf []byte
	pos int
}

func (r *bsonReader) readByte() (byte, error) {
	if r.pos+1 > len(r.buf) {
		return 0, errBSON("underflow: byte")
	}
	b := r.buf[r.pos]
	r.pos++
	return b, nil
}

func (r *bsonReader) readBytes(n int) ([]byte, error) {
	if n < 0 || r.pos+n > len(r.buf) {
		return nil, errBSON("underflow: bytes")
	}
	b := r.buf[r.pos : r.pos+n]
	r.pos += n
	return b, nil
}

func (r *bsonReader) readInt32() (int32, error) {
	b, err := r.readBytes(4)
	if err != nil {
		return 0, err
	}
	return int32(binary.LittleEndian.Uint32(b)), nil
}

func (r *bsonReader) readInt64() (int64, error) {
	b, err := r.readBytes(8)
	if err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint64(b)), nil
}

// readCString reads a NUL-terminated string (the NUL is consumed, not returned).
func (r *bsonReader) readCString() (string, error) {
	for i := r.pos; i < len(r.buf); i++ {
		if r.buf[i] == 0x00 {
			s := string(r.buf[r.pos:i])
			r.pos = i + 1
			return s, nil
		}
	}
	return "", errBSON("unterminated cstring")
}

// readString reads a BSON string: int32 length (INCLUDES the trailing NUL) +
// bytes + NUL. The returned string excludes the NUL.
func (r *bsonReader) readString() (string, error) {
	n, err := r.readInt32()
	if err != nil {
		return "", err
	}
	if n < 1 {
		return "", errBSON("invalid string length")
	}
	b, err := r.readBytes(int(n))
	if err != nil {
		return "", err
	}
	return string(b[:n-1]), nil // strip the trailing NUL
}

// parseBSON parses a single top-level BSON document from buf (the test entry
// point; production decode calls parseDocument directly so the codec can size a
// trailing returnFieldsSelector via remaining()). Trailing bytes after the
// document terminator are an error — the document length must account for the
// whole slice.
func parseBSON(buf []byte) (bsonDoc, error) {
	r := &bsonReader{buf: buf}
	d, err := parseDocument(r)
	if err != nil {
		return bsonDoc{}, err
	}
	if r.pos != len(buf) {
		return bsonDoc{}, errBSON("trailing bytes after document")
	}
	return d, nil
}

// parseDocument walks one document: int32 docLength (INCLUDES itself + the
// trailing 0x00) + elements + 0x00. Nested documents recurse (Task 5).
func parseDocument(r *bsonReader) (bsonDoc, error) {
	start := r.pos
	docLen, err := r.readInt32()
	if err != nil {
		return bsonDoc{}, err
	}
	if docLen < 5 { // 4 length bytes + at least the 1-byte terminator
		return bsonDoc{}, errBSON("document length too small")
	}
	end := start + int(docLen)
	if end > len(r.buf) {
		return bsonDoc{}, errBSON("document length exceeds buffer")
	}
	var d bsonDoc
	for {
		if r.pos >= end {
			return bsonDoc{}, errBSON("document not terminated")
		}
		t, err := r.readByte()
		if err != nil {
			return bsonDoc{}, err
		}
		if t == 0x00 { // terminator
			if r.pos != end {
				return bsonDoc{}, errBSON("document terminator misplaced")
			}
			return d, nil
		}
		name, err := r.readCString()
		if err != nil {
			return bsonDoc{}, err
		}
		val, err := parseElementValue(r, t)
		if err != nil {
			return bsonDoc{}, err
		}
		d.elems = append(d.elems, bsonElem{name: name, typ: t, val: val})
	}
}

// parseElementValue decodes one element value by type. Part 1 handles the
// fixed-width scalar types; Part 2 (Task 5) adds the variable-length + nested
// cases (String 0x02, Document 0x03, Array 0x04, Binary 0x05, ObjectId 0x07,
// Regex 0x0B, Symbol 0x0E). ANY other type byte → throw (upstream parity).
func parseElementValue(r *bsonReader, t byte) (any, error) {
	switch t {
	case 0x01: // Double
		b, err := r.readBytes(8)
		if err != nil {
			return nil, err
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(b)), nil
	case 0x08: // Boolean
		b, err := r.readByte()
		if err != nil {
			return nil, err
		}
		return b != 0x00, nil
	case 0x09: // Datetime (int64 ms)
		return r.readInt64()
	case 0x0A: // Null (no bytes)
		return nil, nil
	case 0x10: // Int32
		return r.readInt32()
	case 0x11: // Timestamp (int64)
		return r.readInt64()
	case 0x12: // Int64
		return r.readInt64()
	case 0x02: // String
		return r.readString()
	case 0x0E: // Symbol (same wire shape as String)
		return r.readString()
	case 0x03: // Document (recurse)
		return parseDocument(r)
	case 0x04: // Array (recurse — same wire shape as Document)
		return parseDocument(r)
	case 0x05: // Binary: int32 len + subtype byte + bytes
		n, err := r.readInt32()
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, errBSON("invalid binary length")
		}
		if _, err := r.readByte(); err != nil { // subtype
			return nil, err
		}
		return r.readBytes(int(n))
	case 0x07: // ObjectId (12 bytes)
		return r.readBytes(12)
	case 0x0B: // Regex: two cstrings (pattern + options)
		pat, err := r.readCString()
		if err != nil {
			return nil, err
		}
		opt, err := r.readCString()
		if err != nil {
			return nil, err
		}
		return [2]string{pat, opt}, nil
	default:
		// Part 2 (Task 5) inserts the variable-length cases ABOVE this default.
		return nil, errBSON("invalid BSON element type")
	}
}
