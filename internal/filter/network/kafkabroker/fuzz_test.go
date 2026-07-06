package kafkabroker

import (
	"bytes"
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

// FuzzKafkaDecode is the 40th fuzzer (SPEC §15.1 Layer C). The Kafka decoder is
// direction-aware but a SINGLE decoder owns both pumps, so one fuzzer drives BOTH
// directions: decodeOnData (request side, high-water) and decodeOnWrite (response
// side, incremental). It feeds arbitrary bytes through both production entry
// points and asserts:
//  1. no panic (a panic fails the fuzz run);
//  2. the input slice (the chain-buffer stand-in) is NEVER mutated (R1, both
//     directions) — captured-copy compare;
//  3. both private reassembly buffers (readBuf, writeBuf) stay bounded — no
//     unbounded growth on partial-frame input (a partial frame never completes,
//     so it sits in the buffer, but the buffer may never exceed the cumulative
//     bytes fed to that direction).
func FuzzKafkaDecode(f *testing.F) {
	// Request-side seeds: a valid ApiVersions(18) v0 frame, an unknown-key frame
	// (api_key 9999), a partial frame (length prefix declaring more than present),
	// an oversized length (huge declared N), a malformed-client_id frame
	// (client_id length -5).
	f.Add(buildRequest(18, 0, 7, "c", false))                      // valid ApiVersions(18) v0
	f.Add(buildRequest(9999, 0, 8, "c", false))                    // unknown api_key
	f.Add(buildRequest(18, 0, 9, "client", false)[:6])             // partial frame (declares more)
	f.Add(append(beI32(1<<28), []byte{0x00, 0x12, 0x00, 0x00}...)) // oversized declared length
	f.Add(buildRequestBadClientID(18, 0, 10, -5))                  // malformed client_id length -5

	// Response-side seeds: a correlated response frame (len + correlation_id) and
	// an unregistered/garbage response frame (a 1-byte body that cannot hold an
	// INT32 correlation_id → response.failure).
	f.Add(buildResponse(7))                  // 4-byte len + correlation_id
	f.Add(append(beI32(1), []byte{0xFF}...)) // garbage/unregistered response (truncated corr)

	f.Fuzz(func(t *testing.T, data []byte) {
		reg := stats.NewRegistry()
		ks := newKafkaStats(reg, "fuzz")
		d := newDecoder(ks)

		orig := append([]byte(nil), data...)

		// Invariant 1: no panic (implicit). Feed BOTH directions through the one
		// decoder — the same bytes drive the request and response decode paths.
		// decodeOnData is high-water (pass totalAppended == len(data) for a single
		// feed); decodeOnWrite is incremental.
		d.decodeOnData(data, int64(len(data)))
		d.decodeOnWrite(data)

		// Second cumulative round on the read side (high-water: totalAppended must
		// grow for the new bytes to be observed) + a second incremental write round.
		doubled := append(append([]byte(nil), data...), data...)
		d.decodeOnData(doubled, int64(len(doubled)))
		d.decodeOnWrite(data)

		// Invariant 2: the input was never mutated (R1, both directions).
		if !bytes.Equal(data, orig) {
			t.Fatal("decode mutated the chain bytes")
		}

		// Invariant 3: both private buffers stay bounded. The read side saw at most
		// the cumulative high-water bytes (len(doubled)); the write side has no
		// high-water mark (28.2 asymmetry) and saw `data` twice, so at most
		// 2*len(data). A small slack covers nothing here — a complete frame is
		// consumed (sliced out), so a buffer never EXCEEDS what was appended.
		if len(d.readBuf) > len(doubled) || len(d.writeBuf) > 2*len(data) {
			t.Fatalf("a private buffer grew unboundedly: read=%d (cap %d) write=%d (cap %d)",
				len(d.readBuf), len(doubled), len(d.writeBuf), 2*len(data))
		}
	})
}
