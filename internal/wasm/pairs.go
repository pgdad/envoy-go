// Byte-faithful reimplementation of the proxy-wasm pairs wire format per
// R3 + parent §13-R3.
//
// Semantic reference: proxy-wasm-cpp-host:include/proxy-wasm/pairs_util.h +
// src/pairs_util.cc at the AMEND-A6 pinned SHA da3ce05d. This file mirrors
// the wire layout produced by PairsUtil::marshalPairs and consumed by
// PairsUtil::toPairs.
//
// Wire format (little-endian throughout — wazero runs only on
// little-endian hosts in the envoy-go deployment surface, matching the
// upstream `usesWasmByteOrder == false` non-wasm-byte-order codepath which
// applies the identity transform):
//
//	u32 num_pairs
//	(u32 key_len, u32 value_len) * num_pairs
//	(key_bytes, NUL, value_bytes, NUL) * num_pairs
//
// Encode is total: any []HeaderPair (including nil) round-trips. Decode is
// strict: malformed inputs (truncated, missing NUL terminators, declared
// lengths overrunning the buffer, trailing garbage) return a wrapped error
// + nil pairs slice. The upstream cpp returns an empty Pairs vector on
// malformed input; Go's idiomatic equivalent is the error-return discipline
// chosen here. See Task 3 deviation note in PROGRESS.md.

package wasm

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// HeaderPair is a single (key, value) pair in a header-map serialization.
// Re-exported by registration.go (Task 7) for the ABICallbacks interface.
type HeaderPair struct {
	Key   string
	Value string
}

// errPairsTruncated is the sentinel returned when DecodePairs cannot
// satisfy a fixed-size read against the remaining input. Wrapped via
// fmt.Errorf at each call-site so callers can errors.Is() it.
var errPairsTruncated = errors.New("pairs: truncated buffer")

// errPairsTrailingBytes is the sentinel returned when DecodePairs consumes
// a well-formed pairs frame but the buffer has unread trailing bytes —
// upstream's `pos != end` rejection per src/pairs_util.cc.
var errPairsTrailingBytes = errors.New("pairs: trailing bytes after well-formed frame")

// errPairsMissingNUL is the sentinel returned when a pair body lacks the
// expected NUL terminator after the length-prefixed key or value — upstream's
// `*pos++ != '\0'` rejection per src/pairs_util.cc.
var errPairsMissingNUL = errors.New("pairs: missing NUL terminator")

// EncodePairs serializes pairs into the proxy-wasm pairs wire format:
//
//	u32 num_pairs
//	u32 key_len, u32 value_len (repeated num_pairs times)
//	key_bytes NUL value_bytes NUL (repeated num_pairs times)
//
// All u32 little-endian per proxy-wasm-cpp-host:src/pairs_util.cc. The
// output buffer is single-shot allocated at the exact total size returned
// by encodedSize — no incremental growth.
func EncodePairs(pairs []HeaderPair) []byte {
	out := make([]byte, encodedSize(pairs))

	// num_pairs at offset 0.
	binary.LittleEndian.PutUint32(out[0:4], uint32(len(pairs)))
	pos := 4

	// Length-pairs table: (key_len, value_len) repeated num_pairs times.
	for i := range pairs {
		binary.LittleEndian.PutUint32(out[pos:pos+4], uint32(len(pairs[i].Key)))
		pos += 4
		binary.LittleEndian.PutUint32(out[pos:pos+4], uint32(len(pairs[i].Value)))
		pos += 4
	}

	// Bodies: (key_bytes, NUL, value_bytes, NUL) repeated num_pairs times.
	for i := range pairs {
		pos += copy(out[pos:], pairs[i].Key)
		out[pos] = 0x00
		pos++
		pos += copy(out[pos:], pairs[i].Value)
		out[pos] = 0x00
		pos++
	}

	return out
}

// encodedSize returns the exact byte size of the EncodePairs(pairs) output.
// Mirrors upstream PairsUtil::pairsSize: 4 bytes for num_pairs plus, per
// pair, 8 bytes of length-pair + key_len + 1 NUL + value_len + 1 NUL.
func encodedSize(pairs []HeaderPair) int {
	size := 4 // num_pairs u32
	for i := range pairs {
		size += 8                       // (key_len u32, value_len u32)
		size += len(pairs[i].Key) + 1   // key bytes + NUL
		size += len(pairs[i].Value) + 1 // value bytes + NUL
	}
	return size
}

// DecodePairs deserializes the proxy-wasm pairs wire format. Returns a
// wrapped error on: truncated buffer; oversize pair count; key-len or
// value-len overruns the remaining buffer; missing NUL terminator after
// key or value; trailing bytes after the last well-formed pair (strict
// total-size check, mirroring upstream's pos != end rejection).
//
// On error the returned pairs slice is nil (matching upstream's empty-vector
// disposition adapted to Go's idiomatic error pattern).
func DecodePairs(buf []byte) ([]HeaderPair, error) {
	if len(buf) < 4 {
		return nil, fmt.Errorf("read num_pairs: %w (need 4 bytes, have %d)", errPairsTruncated, len(buf))
	}

	numPairs := binary.LittleEndian.Uint32(buf[0:4])
	pos := 4

	// Defensive cap: each pair requires at least 8 (length-pair) + 2 (two
	// NUL terminators) = 10 bytes minimum. Reject before we attempt the
	// per-pair length-table read if num_pairs * 10 would already exceed
	// the remaining buffer. Mirrors upstream's PROXY_WASM_HOST_PAIRS_MAX_COUNT
	// + early-bound check at src/pairs_util.cc.
	remaining := len(buf) - pos
	if uint64(numPairs) > uint64(remaining/10)+1 && uint64(numPairs)*10 > uint64(remaining) {
		return nil, fmt.Errorf("num_pairs=%d exceeds buffer capacity (remaining=%d): %w", numPairs, remaining, errPairsTruncated)
	}

	// Length-pairs table: 8 bytes per pair (u32 key_len + u32 value_len).
	if uint64(numPairs)*8 > uint64(remaining) {
		return nil, fmt.Errorf("length-pairs table for num_pairs=%d overruns buffer (need %d bytes, have %d): %w", numPairs, numPairs*8, remaining, errPairsTruncated)
	}

	type sizes struct {
		keyLen   uint32
		valueLen uint32
	}
	table := make([]sizes, numPairs)
	for i := uint32(0); i < numPairs; i++ {
		table[i].keyLen = binary.LittleEndian.Uint32(buf[pos : pos+4])
		pos += 4
		table[i].valueLen = binary.LittleEndian.Uint32(buf[pos : pos+4])
		pos += 4
	}

	// Bodies: per pair, key_len bytes + NUL + value_len bytes + NUL.
	// Validate each step against remaining bytes; mirror upstream's
	// `pos + s.first + 1 > end` and `pos + s.second + 1 > end` bounds.
	pairs := make([]HeaderPair, numPairs)
	for i := uint32(0); i < numPairs; i++ {
		keyLen := table[i].keyLen
		valueLen := table[i].valueLen

		// Key body + NUL.
		if uint64(pos)+uint64(keyLen)+1 > uint64(len(buf)) {
			return nil, fmt.Errorf("pair[%d] key body+NUL (key_len=%d) overruns buffer (pos=%d total=%d): %w", i, keyLen, pos, len(buf), errPairsTruncated)
		}
		key := string(buf[pos : pos+int(keyLen)])
		pos += int(keyLen)
		if buf[pos] != 0x00 {
			return nil, fmt.Errorf("pair[%d] key terminator: %w (got %#x at offset %d)", i, errPairsMissingNUL, buf[pos], pos)
		}
		pos++

		// Value body + NUL.
		if uint64(pos)+uint64(valueLen)+1 > uint64(len(buf)) {
			return nil, fmt.Errorf("pair[%d] value body+NUL (value_len=%d) overruns buffer (pos=%d total=%d): %w", i, valueLen, pos, len(buf), errPairsTruncated)
		}
		value := string(buf[pos : pos+int(valueLen)])
		pos += int(valueLen)
		if buf[pos] != 0x00 {
			return nil, fmt.Errorf("pair[%d] value terminator: %w (got %#x at offset %d)", i, errPairsMissingNUL, buf[pos], pos)
		}
		pos++

		pairs[i] = HeaderPair{Key: key, Value: value}
	}

	// Strict total-size check — upstream rejects pos != end. Trailing
	// garbage indicates a corrupted frame (or a length-pair-table that
	// under-promised the body bytes).
	if pos != len(buf) {
		return nil, fmt.Errorf("%w: consumed=%d total=%d", errPairsTrailingBytes, pos, len(buf))
	}

	if numPairs == 0 {
		return nil, nil
	}
	return pairs, nil
}
