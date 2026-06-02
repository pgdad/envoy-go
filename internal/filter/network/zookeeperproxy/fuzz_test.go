package zookeeperproxy

import (
	"bytes"
	"testing"

	zookeeper_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/stats"
)

// FuzzZookeeperRequestDecode is the 37th fuzzer (parent §11.10 / SPEC §15.1
// Layer C). It feeds arbitrary bytes through the production decodeOnData entry
// point and asserts the three safety invariants:
//  1. no panic;
//  2. the input slice (the chain buffer stand-in) is NEVER mutated (R3);
//  3. the decoder-internal reassembly buffer stays bounded by
//     max_packet_bytes + the frame-header overhead (no unbounded growth).
//
// Invariant-3 reasoning: decodeOnData processes frames in a loop until no
// complete frame can be extracted. After the loop, readBuf holds at most one
// partial frame: a 4-byte length prefix (claiming exactly maxPkt bytes of
// payload) with maxPkt-1 body bytes = maxPkt+3 bytes total. A length prefix
// ≥ 0 that exceeds maxPkt triggers decoderError (readBuf=nil). Therefore the
// post-call bound is maxPkt+4 bytes; the +8 slack accommodates any future
// frame-header growth without invalidating the test.
func FuzzZookeeperRequestDecode(f *testing.F) {
	// Seed corpus: a valid connect frame, a ping, a data request, garbage, an
	// oversized length prefix, and a partial frame.
	f.Add(connectFrame(nil))
	f.Add(zkFrame(be32(pingXid), be32(opPing)))
	f.Add(dataFrame(1, opGetData, []byte("/path")))
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0x00})
	f.Add(append(be32(1<<20), make([]byte, 16)...)) // length prefix exceeds maxPkt (1024)
	f.Add(dataFrame(1, opCreate, nil)[:6])          // partial frame (6 bytes < 4+prefix minimum)

	f.Fuzz(func(t *testing.T, data []byte) {
		const maxPkt = 1024 // small bound so the invariant is exercised by short inputs
		reg := stats.NewRegistry()
		cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{
			StatPrefix:     "fuzz",
			MaxPacketBytes: wrapperspb.UInt32(maxPkt),
		})
		if err != nil {
			t.Fatal(err)
		}
		rs := newRosterStats(reg, "fuzz")
		d := newRequestDecoder(cfg, rs)

		orig := append([]byte(nil), data...)

		// Invariant 1: no panic (implicit — a panic fails the fuzz run).
		d.decodeOnData(data, int64(len(data)))
		// Feed cumulatively a second time (the chain buffer accumulates) — the
		// high-water mark must not re-process or panic.
		doubled := append(append([]byte(nil), data...), data...)
		d.decodeOnData(doubled, int64(len(doubled)))

		// Invariant 2: the input was never mutated (R3).
		if !bytes.Equal(data, orig) {
			t.Fatal("decodeOnData mutated the chain bytes")
		}

		// Invariant 3: the internal reassembly buffer is bounded.
		// After decodeOnData returns, readBuf holds at most one partial frame:
		// a 4-byte prefix + up to maxPkt-1 body bytes = maxPkt+3 bytes.
		// We use maxPkt+8 to give slack for any future framing changes.
		if len(d.readBuf) > maxPkt+8 {
			t.Fatalf("readBuf grew to %d bytes, want <= max_packet_bytes(%d)+8", len(d.readBuf), maxPkt)
		}
	})
}
