package mongoproxy

import (
	"bytes"
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

// FuzzMongoDecode is the 39th fuzzer (SPEC §15.1 Layer C). The decoder is
// direction-agnostic, so ONE fuzzer drives BOTH directions (decodeOnData for the
// request side, decodeOnWrite for the response side — no 40th fuzzer; D-P6). It
// feeds arbitrary bytes through both production entry points and asserts:
//  1. no panic (a panic fails the fuzz run);
//  2. the input slice (the chain-buffer stand-in) is NEVER mutated (R3, both
//     directions);
//  3. sniffing-off idempotence — once decoding_error fires (sniffing=false), a
//     subsequent feed on EITHER direction decodes/increments NOTHING
//     (decoding_error stays put; sniffing is direction-shared — AMEND-B6 /
//     D-S29.1-6);
//  4. both private buffers (readBuf, writeBuf) stay bounded (no unbounded growth
//     on partial-frame input).
func FuzzMongoDecode(f *testing.F) {
	// Seed corpus: a valid OP_QUERY, an OP_MSG (→ decoding_error), a partial
	// header, an oversized messageLength, a garbage-BSON OP_QUERY, an OP_INSERT.
	f.Add(msgSeed(1, 2004, opQueryBody("db.c", 0, simpleQuery())))
	f.Add(msgSeed(1, 2013, nil))
	f.Add(msgSeed(1, 2004, opQueryBody("db.c", 0, simpleQuery()))[:10])
	f.Add(append(leI32(1<<20), make([]byte, 12)...))
	f.Add(msgSeed(1, 2004, opQueryBody("db.c", 0, []byte{0x05, 0x00, 0x00, 0x00, 0x13}))) // bad BSON type
	f.Add(msgSeed(1, 2002, append(leI32(0), append([]byte("db.c\x00"), simpleQuery()...)...)))

	// Response-side seeds: a valid empty OP_REPLY, an OP_COMMANDREPLY (2011), an
	// OP_REPLY whose numberReturned LIES (claims 5 docs, none follow — malformed).
	f.Add(respSeed(1, 1, replyBodySeed(0, 0, 0)))
	f.Add(respSeed(1, 2011, append(docSeed(), docSeed()...)))
	f.Add(respSeed(1, 1, replyBodySeed(0, 0, 5))) // numberReturned=5, no docs

	f.Fuzz(func(t *testing.T, data []byte) {
		reg := stats.NewRegistry()
		cfg := &compiledConfig{statPrefix: "fuzz", commands: map[string]bool{"isMaster": true}}
		ms := newMongoStats(reg, "fuzz")
		cfg.stats = ms
		d := newDecoder(cfg, ms)

		orig := append([]byte(nil), data...)

		// Invariant 1: no panic (implicit). Feed BOTH directions through the one
		// decoder — the same bytes drive the request and response decode paths.
		d.decodeOnData(data, int64(len(data)))
		d.decodeOnWrite(data)
		errAfterFirst := ms.counters["decoding_error"].Load()
		sniffingAfterFirst := d.sniffing.Load()

		// Second cumulative round on the read side + a second write round.
		doubled := append(append([]byte(nil), data...), data...)
		d.decodeOnData(doubled, int64(len(doubled)))
		d.decodeOnWrite(data)

		// Invariant 2: the input was never mutated (R3, both directions).
		if !bytes.Equal(data, orig) {
			t.Fatal("decode mutated the chain bytes")
		}

		// Invariant 3: once sniffing is off, decoding_error never increments again
		// on EITHER direction (direction-shared at-most-once).
		if !sniffingAfterFirst && ms.counters["decoding_error"].Load() != errAfterFirst {
			t.Fatalf("decoding_error grew after sniffing-off: %d → %d",
				errAfterFirst, ms.counters["decoding_error"].Load())
		}

		// Invariant 4: both private buffers stay bounded — at most the cumulative
		// bytes fed to that direction (no growth beyond what was appended). The
		// read side saw at most len(doubled); the write side saw data twice
		// (decodeOnWrite has no high-water mark — 28.2 asymmetry), so at most
		// 2*len(data). Once sniffing is off each buffer is nil.
		if len(d.readBuf) > len(doubled)+16 || len(d.writeBuf) > 2*len(data)+16 {
			t.Fatalf("a private buffer grew unboundedly: read=%d write=%d", len(d.readBuf), len(d.writeBuf))
		}
	})
}

// msgSeed mirrors the codec_test msg() helper (a separate name avoids cross-file
// fuzz/seed coupling; identical layout). 16-byte LE header + body.
func msgSeed(reqID, opCode int32, body []byte) []byte { return msg(reqID, opCode, body) }

// respSeed builds a response wire frame (16-byte LE header, responseTo=0).
func respSeed(reqID, opCode int32, body []byte) []byte { return msg(reqID, opCode, body) }

// replyBodySeed builds an OP_REPLY body: flags + cursorID + startingFrom +
// numberReturned (+ no docs — numReturned may LIE for the malformed seed).
func replyBodySeed(flags int32, cursorID int64, numReturned int32) []byte {
	out := append(leI32(flags), leI64(cursorID)...)
	out = append(out, leI32(0)...)
	return append(out, leI32(numReturned)...)
}

// docSeed is a minimal empty BSON document (5 bytes).
func docSeed() []byte { return doc() }
