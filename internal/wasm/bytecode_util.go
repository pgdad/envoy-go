// Byte-faithful ABI-version detection for proxy-wasm modules per AMEND-A6.
//
// Semantic reference: proxy-wasm-cpp-host:src/bytecode_util.cc:32-97 at the
// AMEND-A6 pinned SHA da3ce05d. The implementation scans the wasm module's
// export section (section ID 7) for a function-kind export named
// `proxy_abi_version_0_2_1` (or `_0_2_0` / `_0_1_0`) and returns the matching
// AbiVersion sentinel. Wire-format reference:
// https://webassembly.github.io/spec/core/binary/modules.html
//
// Wire-protocol invariants reproduced verbatim:
//   - Module header: 4-byte magic 0x00 0x61 0x73 0x6d ("\0asm") + 4-byte LE
//     version (which getAbiVersion does NOT validate beyond magic; the upstream
//     checkWasmHeader only compares the 4 magic bytes).
//   - Each section: 1-byte section ID + unsigned LEB128 section size + payload.
//   - Export section ID = 7.
//   - Export vector: LEB128 count + count-many export entries.
//   - Export entry: LEB128 name length + name bytes + 1-byte kind (0=function,
//     1=table, 2=memory, 3=global) + LEB128 index.
//   - Sentinel must be function-kind (0x00) to count; same-named globals/
//     tables/memories do NOT yield a positive detection.
//   - Multiple sentinel exports: first hit wins (cpp returns on first match).
//
// The implementation does NOT depend on wazero — it operates on the raw byte
// slice. This is intentional: the same detection runs before wazero compile is
// invoked (Task 5 compile.go calls GetAbiVersion to gate the v0.2.1-only
// acceptance per AMEND-A6 before paying the compile cost).

package wasm

import (
	"bytes"
	"fmt"
)

// AbiVersion is the proxy-wasm ABI version sentinel detected from the
// module's export section per AMEND-A6. The byte-detection result; NOT a
// wire-protocol enum (those live in internal/wasm/abi).
type AbiVersion int

// AbiVersion sentinel values. Only AbiVersion_0_2_1 is accepted by envoy-go
// at phase 25.1 per AMEND-A6; the other detected variants exist so that
// Task 5 compile.go can emit a precise PARSE-REJECT diagnostic naming the
// guest's actual ABI version when it is one of the older known versions.
const (
	// AbiVersionUnknown is the sentinel returned when no proxy_abi_version_*
	// function-kind export is present in the module's export section (and
	// the module bytes are otherwise well-formed enough for the scan to
	// complete).
	AbiVersionUnknown AbiVersion = iota
	// AbiVersion_0_1_0 corresponds to the `proxy_abi_version_0_1_0`
	// sentinel export.
	AbiVersion_0_1_0
	// AbiVersion_0_2_0 corresponds to the `proxy_abi_version_0_2_0`
	// sentinel export.
	AbiVersion_0_2_0
	// AbiVersion_0_2_1 corresponds to the `proxy_abi_version_0_2_1`
	// sentinel export — the only version envoy-go accepts at 25.1.
	AbiVersion_0_2_1
)

// wasmMagic is the 4-byte module-magic prefix every valid wasm binary begins
// with. The version 4 bytes following are NOT verified here (mirrors upstream
// checkWasmHeader which compares only the 4 magic bytes per src/bytecode_util.cc:25-28).
var wasmMagic = []byte{0x00, 0x61, 0x73, 0x6d}

// exportSectionID is the section-ID byte assigned to the export section
// in the wasm binary format.
const exportSectionID byte = 0x07

// exportKindFunction is the 1-byte kind tag for a function-kind export
// (vs. table=1, memory=2, global=3). The sentinel must be function-kind to
// count per upstream src/bytecode_util.cc:73-89.
const exportKindFunction byte = 0x00

// Sentinel export names (each is 23 ASCII bytes). Exact-equality byte
// comparison per upstream src/bytecode_util.cc:73-89 — not case-insensitive,
// not prefix-match.
var (
	sentinel010 = []byte("proxy_abi_version_0_1_0")
	sentinel020 = []byte("proxy_abi_version_0_2_0")
	sentinel021 = []byte("proxy_abi_version_0_2_1")
)

// GetAbiVersion scans the wasm module's export section (section ID 7) for
// a function-kind export named `proxy_abi_version_0_2_1` /
// `proxy_abi_version_0_2_0` / `proxy_abi_version_0_1_0` and returns the
// matching AbiVersion. Returns AbiVersionUnknown if no sentinel is present
// (module otherwise well-formed). Returns a wrapped error on malformed
// input (truncated module, bogus section header, invalid LEB128, wrong
// magic, etc.).
//
// Byte-faithful reimplementation of proxy-wasm-cpp-host:src/bytecode_util.cc:32-97
// per AMEND-A6 + 25.1 SPEC §13-R2. The scan stops on first sentinel match (cpp
// returns mid-loop on first match per src/bytecode_util.cc:75-86).
func GetAbiVersion(src []byte) (AbiVersion, error) {
	// Upstream checkWasmHeader: size >= 8 && first 4 bytes are the magic.
	// The cpp implementation tolerates size < 8 by treating it as "no header"
	// (it returns true for size < 8 in checkWasmHeader); but getAbiVersion
	// then steps `pos = data+8` past the end and the while-loop exits with
	// the function returning true + ret==Unknown. We instead REJECT
	// short/empty input outright — that's a strictly tighter contract that
	// matches envoy-go's "wrapped error on malformed input" expectation per
	// Task 2 acceptance criteria.
	if len(src) < 8 {
		return AbiVersionUnknown, fmt.Errorf("wasm header too short: got %d bytes, want >= 8", len(src))
	}
	if !bytes.Equal(src[:4], wasmMagic) {
		return AbiVersionUnknown, fmt.Errorf("wasm magic mismatch: got %#x, want %#x", src[:4], wasmMagic)
	}

	// Skip the 8-byte header (magic + version); the version 4 bytes are
	// NOT validated here (upstream checkWasmHeader does not validate them).
	pos := 8

	for pos < len(src) {
		// Section header: 1-byte section ID + unsigned LEB128 section length.
		sectionType := src[pos]
		pos++

		sectionLen, n, err := readUleb128(src[pos:])
		if err != nil {
			return AbiVersionUnknown, fmt.Errorf("section %d header: %w", sectionType, err)
		}
		pos += n
		// Bound check: section payload must fit within the remaining bytes.
		if uint64(pos)+uint64(sectionLen) > uint64(len(src)) {
			return AbiVersionUnknown, fmt.Errorf("section %d payload overruns module: pos=%d len=%d total=%d", sectionType, pos, sectionLen, len(src))
		}

		if sectionType != exportSectionID {
			// Skip non-export sections per upstream src/bytecode_util.cc:96.
			pos += int(sectionLen)
			continue
		}

		// We are inside the export section: parse the export vector.
		// `sectionEnd` is the byte offset one past the last byte that
		// belongs to this section (a hard bound for all inner reads).
		sectionEnd := pos + int(sectionLen)

		vectorLen, n, err := readUleb128(src[pos:sectionEnd])
		if err != nil {
			return AbiVersionUnknown, fmt.Errorf("export vector length: %w", err)
		}
		pos += n

		for i := uint32(0); i < vectorLen; i++ {
			// Each entry: LEB128 name-len + name bytes + 1-byte kind +
			// LEB128 index. Bounds-check every step against sectionEnd
			// (upstream uses section_end as the inner bound per src/bytecode_util.cc:56-62).
			nameLen, n, err := readUleb128(src[pos:sectionEnd])
			if err != nil {
				return AbiVersionUnknown, fmt.Errorf("export[%d] name length: %w", i, err)
			}
			pos += n
			if uint64(pos)+uint64(nameLen) > uint64(sectionEnd) {
				return AbiVersionUnknown, fmt.Errorf("export[%d] name overruns section: pos=%d nameLen=%d sectionEnd=%d", i, pos, nameLen, sectionEnd)
			}
			nameStart := pos
			pos += int(nameLen)

			// Read the 1-byte export kind. Bound-check against sectionEnd
			// (tighter than upstream's MODULE-end bound at src/bytecode_util.cc:69;
			// the section bound is mandatory here to keep the subsequent
			// readUleb128(src[pos:sectionEnd]) safe under attacker-controlled
			// vectorLen that overstates the entry count + truncated section
			// payload — discovered by Phase 25.1 Task 14 FuzzWasmConfigParse
			// at corpus seed 444839f772f59a6d).
			if pos+1 > sectionEnd {
				return AbiVersionUnknown, fmt.Errorf("export[%d] kind byte overruns section: pos=%d sectionEnd=%d", i, pos, sectionEnd)
			}
			kind := src[pos]
			pos++

			if kind == exportKindFunction {
				name := src[nameStart : nameStart+int(nameLen)]
				// Order matches upstream src/bytecode_util.cc:74-86: check
				// v0.1.0 first, then v0.2.0, then v0.2.1. First match wins.
				if bytes.Equal(name, sentinel010) {
					return AbiVersion_0_1_0, nil
				}
				if bytes.Equal(name, sentinel020) {
					return AbiVersion_0_2_0, nil
				}
				if bytes.Equal(name, sentinel021) {
					return AbiVersion_0_2_1, nil
				}
			}

			// Skip the export's index LEB128. Upstream reuses the
			// `export_name_size` u32 destination here as a scratch slot
			// per src/bytecode_util.cc:89; we just consume the bytes.
			_, n, err = readUleb128(src[pos:sectionEnd])
			if err != nil {
				return AbiVersionUnknown, fmt.Errorf("export[%d] index: %w", i, err)
			}
			pos += n
		}

		// Reached the end of the export section without finding a sentinel —
		// upstream returns true with ret==Unknown here per src/bytecode_util.cc:93.
		return AbiVersionUnknown, nil
	}

	// No export section in the module — sentinel cannot be present.
	return AbiVersionUnknown, nil
}

// readUleb128 decodes an unsigned LEB128 integer from the head of src and
// returns the value, the number of bytes consumed, and any error. Mirrors
// upstream BytecodeUtil::parseVarint (src/bytecode_util.cc:247-275) — bounded
// to 5 continuation iterations (max 32-bit unsigned varint) and rejects
// overflow.
//
// We deliberately do NOT delegate to encoding/binary.Uvarint despite the
// note in the Task 2 prompt: stdlib Uvarint treats the input as a uint64
// little-endian varint and accepts up to 10 continuation bytes — that
// behavior is permissive in ways the upstream cpp parser is not (which caps
// at 5 bytes / 32-bit and rejects shift==28 && top-nibble>3 overflow per
// cpp lines 259-262 + 268-271). Mirror those constraints byte-faithfully.
func readUleb128(src []byte) (val uint32, consumed int, err error) {
	var shift uint
	var total uint32
	for i := 0; i < len(src); i++ {
		b := src[i]
		v := uint32(b & 0x7f)
		if shift == 28 && v > 3 {
			return 0, 0, fmt.Errorf("LEB128 overflow at byte %d (shift=28, v=%d)", i, v)
		}
		total += v << shift
		if b&0x80 == 0 {
			return total, i + 1, nil
		}
		shift += 7
		if shift > 28 {
			return 0, 0, fmt.Errorf("LEB128 overflow: shift=%d > 28", shift)
		}
	}
	return 0, 0, fmt.Errorf("LEB128 truncated: %d bytes consumed without terminator", len(src))
}
