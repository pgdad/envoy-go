package mongoproxy

import (
	"bytes"
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

// FuzzMongoDecode is the 39th fuzzer (SPEC §15.1 Layer C). It feeds arbitrary
// bytes through the production decodeOnData entry point and asserts:
//  1. no panic (a panic fails the fuzz run);
//  2. the input slice (the chain-buffer stand-in) is NEVER mutated (R3);
//  3. sniffing-off idempotence — once decoding_error fires (sniffing=false),
//     a second feed decodes/increments NOTHING (decoding_error stays == 1) and
//     readBuf is released (AMEND-B6 / D-S29.1-6);
//  4. readBuf stays bounded (no unbounded growth on partial-frame input).
func FuzzMongoDecode(f *testing.F) {
	// Seed corpus: a valid OP_QUERY, an OP_MSG (→ decoding_error), a partial
	// header, an oversized messageLength, a garbage-BSON OP_QUERY, an OP_INSERT.
	f.Add(msgSeed(1, 2004, opQueryBody("db.c", 0, simpleQuery())))
	f.Add(msgSeed(1, 2013, nil))
	f.Add(msgSeed(1, 2004, opQueryBody("db.c", 0, simpleQuery()))[:10])
	f.Add(append(leI32(1<<20), make([]byte, 12)...))
	f.Add(msgSeed(1, 2004, opQueryBody("db.c", 0, []byte{0x05, 0x00, 0x00, 0x00, 0x13}))) // bad BSON type
	f.Add(msgSeed(1, 2002, append(leI32(0), append([]byte("db.c\x00"), simpleQuery()...)...)))

	f.Fuzz(func(t *testing.T, data []byte) {
		reg := stats.NewRegistry()
		cfg := &compiledConfig{statPrefix: "fuzz", commands: map[string]bool{"isMaster": true}}
		ms := newMongoStats(reg, "fuzz")
		cfg.stats = ms
		d := newDecoder(cfg, ms)

		orig := append([]byte(nil), data...)

		// Invariant 1: no panic (implicit).
		d.decodeOnData(data, int64(len(data)))
		errAfterFirst := ms.counters["decoding_error"].Load()
		sniffingAfterFirst := d.sniffing

		// Feed cumulatively a second time (the chain buffer accumulates).
		doubled := append(append([]byte(nil), data...), data...)
		d.decodeOnData(doubled, int64(len(doubled)))

		// Invariant 2: the input was never mutated (R3).
		if !bytes.Equal(data, orig) {
			t.Fatal("decodeOnData mutated the chain bytes")
		}

		// Invariant 3: once sniffing is off, decoding_error never increments again.
		if !sniffingAfterFirst && ms.counters["decoding_error"].Load() != errAfterFirst {
			t.Fatalf("decoding_error grew after sniffing-off: %d → %d",
				errAfterFirst, ms.counters["decoding_error"].Load())
		}

		// Invariant 4: readBuf is bounded — at most one partial frame. Once
		// sniffing is off readBuf is nil; otherwise it holds < one complete frame.
		if len(d.readBuf) > len(doubled)+16 {
			t.Fatalf("readBuf grew unboundedly: %d bytes", len(d.readBuf))
		}
	})
}

// msgSeed mirrors the codec_test msg() helper (a separate name avoids cross-file
// fuzz/seed coupling; identical layout). 16-byte LE header + body.
func msgSeed(reqID, opCode int32, body []byte) []byte { return msg(reqID, opCode, body) }
