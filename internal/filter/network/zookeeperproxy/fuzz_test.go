package zookeeperproxy

import (
	"bytes"
	"testing"

	zookeeper_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/pgdad/envoy-go/internal/stats"
)

// FuzzZookeeperResponseDecode is the 38th fuzzer (parent §11.10 / D-P6 /
// SPEC §6). It feeds arbitrary bytes through the production decodeOnWrite entry
// point on a decoder PRE-LOADED with pending requests (so the correlation paths
// are reachable) and with the latency + per-opcode-response-bytes flags ON (so
// those counter paths are fuzzed too), asserting:
//  1. no panic — in particular, the closed-roster rosterStats.inc can never
//     receive an unknown suffix (the §3.4-item-4 connect_readonly→connect
//     mapping is exactly what this guards);
//  2. writeBuf stays bounded by max_packet_bytes + slack (no unbounded growth
//     — R10, the 37th fuzzer's bounded-reassembly discipline);
//  3. the correlation maps never GROW from response input (responses only
//     erase/pop — R10).
func FuzzZookeeperResponseDecode(f *testing.F) {
	// Seed corpus (SPEC §6): a valid data response, a connect response, a watch
	// event, a control (ping) response, an unknown-xid response, a truncated
	// frame, an oversized frame.
	f.Add(stdRespFrame(1, 1, 0))
	f.Add(connectRespFrame(16))
	f.Add(watchEventFrame("/path"))
	f.Add(stdRespFrame(pingXid, 1, 0))
	f.Add(stdRespFrame(9999, 1, 0))
	f.Add(stdRespFrame(1, 1, 0)[:6])
	f.Add(append(be32(1<<20), make([]byte, 16)...))

	f.Fuzz(func(t *testing.T, data []byte) {
		const maxPkt = 1024 // small bound so the invariant is exercised by short inputs
		reg := stats.NewRegistry()
		cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{
			StatPrefix:                          "fuzzresp",
			MaxPacketBytes:                      wrapperspb.UInt32(maxPkt),
			EnableLatencyThresholdMetrics:       true,
			EnablePerOpcodeResponseBytesMetrics: true,
			EnablePerOpcodeDecoderErrorMetrics:  true,
		})
		if err != nil {
			t.Fatal(err)
		}
		rs := newRosterStats(reg, "fuzzresp")
		d := newDecoder(cfg, rs)

		// Pre-load pending requests so every correlation path is reachable:
		// a data request, a READONLY connect (the §3.4-item-4 trap), a ping.
		feedRequest(d, dataFrame(1, opGetData, padTo(opGetData)))
		ro := true
		feedRequest(d, connectFrame(&ro))
		feedRequest(d, zkFrame(be32(pingXid), be32(opPing)))
		mapsBefore := correlationSize(d)

		orig := append([]byte(nil), data...)

		// Invariant 1: no panic (implicit — a panic fails the fuzz run).
		d.decodeOnWrite(data)
		// Feed a second time (reassembly accumulation across OnWrite calls).
		d.decodeOnWrite(data)

		// The input slice is never mutated (R3).
		if !bytes.Equal(data, orig) {
			t.Fatal("decodeOnWrite mutated the input bytes")
		}

		// Invariant 3: response input never GROWS the correlation maps.
		if got := correlationSize(d); got > mapsBefore {
			t.Fatalf("correlation maps grew from %d to %d entries on response input", mapsBefore, got)
		}

		// Invariant 2: the write-side reassembly buffer is bounded.
		// Post-call invariant: writeBuf holds at most one incomplete frame —
		// a valid length prefix (declared frameLen ≤ maxPkt) with fewer than
		// 4+frameLen bytes present, so len(writeBuf) ≤ maxPkt+3. Oversized
		// frames (frameLen > maxPkt) abandon writeBuf to nil immediately. The
		// +8 slack (matching the request-side fuzzer) accommodates any future
		// framing growth without invalidating the test.
		if len(d.writeBuf) > maxPkt+8 {
			t.Fatalf("writeBuf grew to %d bytes, want <= max_packet_bytes(%d)+8", len(d.writeBuf), maxPkt)
		}
	})
}

// correlationSize returns the total entry count across both correlation
// structures (single-goroutine fuzz context — no lock needed for the read,
// but take it anyway for -race cleanliness under fuzz workers).
func correlationSize(d *decoder) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(d.requestsByXid)
	for _, q := range d.controlRequestsByXid {
		n += len(q)
	}
	return n
}

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
		d := newDecoder(cfg, rs)

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
