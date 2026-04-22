package differential

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// Verdict is the result of comparing two byte streams.
type Verdict struct {
	Equal           bool
	FirstDiffOffset int
	HexDump         string // empty when Equal
}

// CompareBytes is the byte-exact comparator used by phase-00's echo fixture.
// More elaborate comparisons (semantic header diff, stat tree intersection)
// land in later phases as their dimensions are added to BEHAVIOR_CONTRACT.md.
func CompareBytes(ref, subj []byte) (Verdict, error) {
	if len(ref) == len(subj) {
		eq := true
		for i := range ref {
			if ref[i] != subj[i] {
				return Verdict{
					Equal:           false,
					FirstDiffOffset: i,
					HexDump:         hexWindow(ref, subj, i),
				}, nil
			}
		}
		if eq {
			return Verdict{Equal: true}, nil
		}
	}
	// Different lengths: first divergence is at min length.
	off := len(ref)
	if len(subj) < off {
		off = len(subj)
	}
	for i := 0; i < off; i++ {
		if ref[i] != subj[i] {
			return Verdict{Equal: false, FirstDiffOffset: i, HexDump: hexWindow(ref, subj, i)}, nil
		}
	}
	return Verdict{Equal: false, FirstDiffOffset: off, HexDump: hexWindow(ref, subj, off)}, nil
}

func hexWindow(ref, subj []byte, off int) string {
	const window = 32
	start := off - window/2
	if start < 0 {
		start = 0
	}
	end := func(b []byte) int {
		e := off + window/2
		if e > len(b) {
			e = len(b)
		}
		return e
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "first divergence at offset %d\n", off)
	fmt.Fprintf(&sb, "ref [%d..%d]:\n%s\n", start, end(ref), hex.Dump(ref[start:end(ref)]))
	fmt.Fprintf(&sb, "subj[%d..%d]:\n%s", start, end(subj), hex.Dump(subj[start:end(subj)]))
	return sb.String()
}
