package kafkabroker

import (
	"encoding/binary"
	"errors"
	"sync"
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

// newTestDecoder wires a decoder over a fresh roster (stat_prefix "k").
func newTestDecoder(t *testing.T) (*decoder, *kafkaStats) {
	t.Helper()
	reg := stats.NewRegistry()
	ks := newKafkaStats(reg, "k")
	return newDecoder(ks), ks
}

// beI16 / beI32 are big-endian primitive encoders for the test builders.
func beI16(v int16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(v))
	return b
}

func beI32(v int32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(v))
	return b
}

// buildHeader builds a Kafka request header body (NO length prefix):
//
//	api_key INT16, api_version INT16, correlation_id INT32, client_id NULLABLE_STRING,
//	and (iff flexible) an empty tagged-fields section (single 0x00 varint).
func buildHeader(apiKey, apiVersion int16, corr int32, clientID string, flexible bool) []byte {
	out := beI16(apiKey)
	out = append(out, beI16(apiVersion)...)
	out = append(out, beI32(corr)...)
	out = append(out, beI16(int16(len(clientID)))...)
	out = append(out, []byte(clientID)...)
	if flexible {
		out = append(out, 0x00) // empty tagged fields (UNSIGNED_VARINT count 0)
	}
	return out
}

// buildRequest builds a complete request frame: a 4-byte INT32 length prefix +
// the header body produced by buildHeader.
func buildRequest(apiKey, apiVersion int16, corr int32, clientID string, flexible bool) []byte {
	body := buildHeader(apiKey, apiVersion, corr, clientID, flexible)
	return append(beI32(int32(len(body))), body...)
}

// buildRequestNullClientID builds a request frame with a null client_id (len -1).
func buildRequestNullClientID(apiKey, apiVersion int16, corr int32) []byte {
	body := beI16(apiKey)
	body = append(body, beI16(apiVersion)...)
	body = append(body, beI32(corr)...)
	body = append(body, beI16(-1)...) // null client_id
	return append(beI32(int32(len(body))), body...)
}

// buildRequestBadClientID builds a request frame whose client_id length is a
// malformed negative value (< -1) — a within-frame decode failure.
func buildRequestBadClientID(apiKey, apiVersion int16, corr int32, badLen int16) []byte {
	body := beI16(apiKey)
	body = append(body, beI16(apiVersion)...)
	body = append(body, beI32(corr)...)
	body = append(body, beI16(badLen)...) // malformed nullable-string length
	return append(beI32(int32(len(body))), body...)
}

// buildResponse builds a complete Kafka response frame: a 4-byte INT32 length
// prefix + INT32 correlation_id and nothing else. The response side reads ONLY
// correlation_id (D-PLAN-3), so a 4-byte body is sufficient and the length prefix
// covers it (nextFrame requires only a complete 4-byte prefix + N-byte body, and
// the reader reads exactly 4 bytes for the correlation_id).
func buildResponse(corr int32) []byte {
	body := beI32(corr)
	return append(beI32(int32(len(body))), body...)
}

func TestDecodeRequestHeader_KnownKey(t *testing.T) {
	d, ks := newTestDecoder(t)
	frame := buildRequest(18, 0, 7, "c", false) // ApiVersions(18) v0 non-flexible
	d.decodeOnData(frame, int64(len(frame)))
	if got := ks.counters["request.api_versions_request"].Load(); got != 1 {
		t.Fatalf("api_versions_request = %d, want 1", got)
	}
	if spec, ok := d.lookupAndErase(7); !ok || spec.apiKey != 18 || spec.apiVersion != 0 {
		t.Fatalf("correlation not registered: %+v %v", spec, ok)
	}
}

func TestDecodeRequestHeader_UnknownKey(t *testing.T) {
	d, ks := newTestDecoder(t)
	frame := buildRequest(9999, 0, 1, "c", false)
	d.decodeOnData(frame, int64(len(frame)))
	if got := ks.counters["request.unknown"].Load(); got != 1 {
		t.Fatalf("request.unknown = %d, want 1", got)
	}
	if got := ks.counters["request.failure"].Load(); got != 0 {
		t.Fatalf("request.failure = %d, want 0", got)
	}
	// correlation still registered (register happens before classification)
	if _, ok := d.lookupAndErase(1); !ok {
		t.Fatal("correlation not registered for unknown key")
	}
}

func TestDecodeRequestHeader_UnknownVersion(t *testing.T) {
	d, ks := newTestDecoder(t)
	// key 18 (api_versions) max is 4; 0x7FFF is far above → request.unknown.
	// 0x7FFF >= flexibleSince[18] (3), so the header is flexible — build it with
	// an empty tagged-fields section so the header decodes cleanly and the
	// classification (unknown version) is what's exercised, not a decode failure.
	frame := buildRequest(18, 0x7FFF, 2, "c", true)
	d.decodeOnData(frame, int64(len(frame)))
	if got := ks.counters["request.unknown"].Load(); got != 1 {
		t.Fatalf("request.unknown = %d, want 1", got)
	}
	if got := ks.counters["request.api_versions_request"].Load(); got != 0 {
		t.Fatalf("api_versions_request must NOT be counted for unknown version, got %d", got)
	}
}

func TestDecodeRequestHeader_MalformedClientID(t *testing.T) {
	d, ks := newTestDecoder(t)
	frame := buildRequestBadClientID(18, 0, 33, -5)
	d.decodeOnData(frame, int64(len(frame)))
	if got := ks.counters["request.failure"].Load(); got != 1 {
		t.Fatalf("request.failure = %d, want 1", got)
	}
	// register happens BEFORE client_id — the correlation must still be present.
	if spec, ok := d.lookupAndErase(33); !ok || spec.apiKey != 18 || spec.apiVersion != 0 {
		t.Fatalf("malformed client_id must leave correlation registered: %+v %v", spec, ok)
	}
	// no known counter charged
	if got := ks.counters["request.api_versions_request"].Load(); got != 0 {
		t.Fatalf("api_versions_request = %d, want 0 on failure", got)
	}
}

func TestDecodeRequestHeader_FlexibleTaggedFields(t *testing.T) {
	d, ks := newTestDecoder(t)
	// produce(0) v9 is flexible (flexibleSince[0]==9); append empty tagged fields.
	frame := buildRequest(0, 9, 11, "client", true)
	d.decodeOnData(frame, int64(len(frame)))
	if got := ks.counters["request.produce_request"].Load(); got != 1 {
		t.Fatalf("produce_request = %d, want 1 (tagged-field skip failed?)", got)
	}
	if spec, ok := d.lookupAndErase(11); !ok || spec.apiKey != 0 || spec.apiVersion != 9 {
		t.Fatalf("correlation not registered: %+v %v", spec, ok)
	}
}

func TestPrimitives_NullableStringNull(t *testing.T) {
	d, ks := newTestDecoder(t)
	frame := buildRequestNullClientID(18, 0, 5) // null client_id at a known key
	d.decodeOnData(frame, int64(len(frame)))
	if got := ks.counters["request.api_versions_request"].Load(); got != 1 {
		t.Fatalf("null client_id must still count the request, got %d", got)
	}
	if got := ks.counters["request.failure"].Load(); got != 0 {
		t.Fatalf("null client_id must NOT be a failure, got %d", got)
	}
}

func TestFraming_PartialThenComplete(t *testing.T) {
	d, ks := newTestDecoder(t)
	frame := buildRequest(18, 0, 7, "c", false)
	// Feed first 3 bytes (less than the 4-byte length prefix), then the rest.
	d.decodeOnData(frame[:3], 3)
	if got := ks.counters["request.api_versions_request"].Load(); got != 0 {
		t.Fatalf("partial frame must not count yet, got %d", got)
	}
	d.decodeOnData(frame, int64(len(frame)))
	if got := ks.counters["request.api_versions_request"].Load(); got != 1 {
		t.Fatalf("completed frame must count once, got %d", got)
	}
	if got := ks.counters["request.failure"].Load(); got != 0 {
		t.Fatalf("partial frame must not be a failure, got %d", got)
	}
}

func TestFraming_MultiReadNoDoubleCount(t *testing.T) {
	d, ks := newTestDecoder(t)
	f1 := buildRequest(18, 0, 1, "a", false)
	f2 := buildRequest(0, 9, 2, "b", true)
	full := append(append([]byte{}, f1...), f2...)
	// First call: only the first frame plus a partial second frame is visible.
	k := len(f1) + 3
	d.decodeOnData(full[:k], int64(k))
	if got := ks.counters["request.api_versions_request"].Load(); got != 1 {
		t.Fatalf("first frame should count once, got %d", got)
	}
	if got := ks.counters["request.produce_request"].Load(); got != 0 {
		t.Fatalf("second frame partial — should not count yet, got %d", got)
	}
	// Second call: the full cumulative buffer; high-water must not re-count f1.
	d.decodeOnData(full, int64(len(full)))
	if got := ks.counters["request.api_versions_request"].Load(); got != 1 {
		t.Fatalf("first frame double-counted: %d", got)
	}
	if got := ks.counters["request.produce_request"].Load(); got != 1 {
		t.Fatalf("second frame should count once, got %d", got)
	}
}

func TestDecodeReset_ResyncsBuffer(t *testing.T) {
	d, ks := newTestDecoder(t)
	// A complete frame whose body is too short to hold a full header → within-frame
	// short read → failure + buffer reset. Then a valid frame still decodes.
	short := append(beI32(2), []byte{0x00, 0x12}...) // body len 2: only api_key, truncated
	d.decodeOnData(short, int64(len(short)))
	if got := ks.counters["request.failure"].Load(); got != 1 {
		t.Fatalf("short-body frame must be a failure, got %d", got)
	}
	// Feed a fresh valid frame with the cumulative-buffer convention.
	good := buildRequest(18, 0, 9, "c", false)
	full := append(append([]byte{}, short...), good...)
	d.decodeOnData(full, int64(len(full)))
	if got := ks.counters["request.api_versions_request"].Load(); got != 1 {
		t.Fatalf("decoder did not resync after reset, got %d", got)
	}
}

func TestFraming_AbsurdLengthResyncs(t *testing.T) {
	d, ks := newTestDecoder(t)
	// A 4-byte length prefix declaring a body beyond maxFrameLen is FRAMING garbage
	// (N > maxFrameLen): nextFrame resyncs (*buf=nil) and charges NO counter —
	// request.failure is reserved for header-level decode failures on a fully-received
	// frame (D-PLAN-3), not framing corruption.
	absurd := beI32(int32(maxFrameLen + 1)) // declared body > maxFrameLen, no body bytes
	d.decodeOnData(absurd, int64(len(absurd)))
	if got := ks.counters["request.failure"].Load(); got != 0 {
		t.Fatalf("framing garbage must charge no failure, got request.failure=%d", got)
	}
	// Feed a fresh VALID frame with the cumulative-buffer convention; the decoder
	// must have resynced and decode it normally.
	good := buildRequest(18, 0, 1, "c", false) // ApiVersions(18) v0
	full := append(append([]byte{}, absurd...), good...)
	d.decodeOnData(full, int64(len(full)))
	if got := ks.counters["request.api_versions_request"].Load(); got != 1 {
		t.Fatalf("decoder did not resync after absurd length, got api_versions_request=%d", got)
	}
	if got := ks.counters["request.failure"].Load(); got != 0 {
		t.Fatalf("resync + valid frame must charge no failure, got request.failure=%d", got)
	}
}

// ---- per-primitive unit tests ----

func TestReaderInt16(t *testing.T) {
	r := newReader([]byte{0x01, 0x02, 0xFF})
	v, err := r.int16()
	if err != nil || v != 0x0102 {
		t.Fatalf("int16 = %d, %v", v, err)
	}
	if _, err := r.int16(); !errors.Is(err, errShortBuffer) {
		t.Fatalf("truncated int16 should be errShortBuffer, got %v", err)
	}
}

func TestReaderInt32(t *testing.T) {
	r := newReader([]byte{0x00, 0x00, 0x00, 0x2A, 0x01})
	v, err := r.int32()
	if err != nil || v != 42 {
		t.Fatalf("int32 = %d, %v", v, err)
	}
	if _, err := r.int32(); !errors.Is(err, errShortBuffer) {
		t.Fatalf("truncated int32 should be errShortBuffer, got %v", err)
	}
}

func TestReaderNullableString(t *testing.T) {
	// "hi"
	r := newReader(append(beI16(2), []byte("hi")...))
	s, err := r.nullableString()
	if err != nil || string(s) != "hi" {
		t.Fatalf("nullableString = %q, %v", s, err)
	}
	// null (-1)
	r = newReader(beI16(-1))
	s, err = r.nullableString()
	if err != nil || s != nil {
		t.Fatalf("null nullableString = %q, %v, want nil,nil", s, err)
	}
	// malformed length (< -1)
	r = newReader(beI16(-5))
	if _, err := r.nullableString(); !errors.Is(err, errMalformed) {
		t.Fatalf("len<-1 should be errMalformed, got %v", err)
	}
	// length exceeds remaining → errMalformed (complete frame, body too short)
	r = newReader(append(beI16(10), []byte("hi")...))
	if _, err := r.nullableString(); !errors.Is(err, errMalformed) {
		t.Fatalf("len>remaining should be errMalformed, got %v", err)
	}
	// truncated length prefix
	r = newReader([]byte{0x00})
	if _, err := r.nullableString(); !errors.Is(err, errShortBuffer) {
		t.Fatalf("truncated length should be errShortBuffer, got %v", err)
	}
}

func TestReaderUnsignedVarint(t *testing.T) {
	cases := []struct {
		in   []byte
		want uint32
	}{
		{[]byte{0x00}, 0},
		{[]byte{0x01}, 1},
		{[]byte{0x7F}, 127},
		{[]byte{0x80, 0x01}, 128},
		{[]byte{0xAC, 0x02}, 300},
	}
	for _, tc := range cases {
		r := newReader(tc.in)
		v, err := r.unsignedVarint()
		if err != nil || v != tc.want {
			t.Fatalf("unsignedVarint(% x) = %d, %v; want %d", tc.in, v, err, tc.want)
		}
	}
	// truncated continuation
	r := newReader([]byte{0x80})
	if _, err := r.unsignedVarint(); !errors.Is(err, errShortBuffer) {
		t.Fatalf("truncated varint should be errShortBuffer, got %v", err)
	}
}

func TestReaderSkipTaggedFields(t *testing.T) {
	// zero tagged fields
	r := newReader([]byte{0x00})
	if err := r.skipTaggedFields(); err != nil {
		t.Fatalf("empty tagged fields: %v", err)
	}
	// one tagged field: count=1, tag=1, size=3, 3 bytes
	r = newReader([]byte{0x01, 0x01, 0x03, 0xAA, 0xBB, 0xCC, 0x99})
	if err := r.skipTaggedFields(); err != nil {
		t.Fatalf("one tagged field: %v", err)
	}
	if r.remaining() != 1 { // the trailing 0x99 must remain
		t.Fatalf("skipTaggedFields over-consumed: %d remaining", r.remaining())
	}
	// truncated tagged-field body
	r = newReader([]byte{0x01, 0x01, 0x05, 0xAA}) // claims 5 bytes, only 1 present
	if err := r.skipTaggedFields(); !errors.Is(err, errMalformed) {
		t.Fatalf("truncated tagged-field body should be errMalformed, got %v", err)
	}
}

// ---- response-side decode (Task 8) ----

func TestDecodeResponse_Correlated(t *testing.T) {
	d, ks := newTestDecoder(t)
	req := buildRequest(3, 9, 42, "c", true) // metadata(3) v9 (flexible) — registers corr 42
	d.decodeOnData(req, int64(len(req)))
	resp := buildResponse(42) // correlation_id=42
	d.decodeOnWrite(resp)
	if got := ks.counters["response.metadata_response"].Load(); got != 1 {
		t.Fatalf("response.metadata_response = %d, want 1", got)
	}
	if got := ks.counters["response.failure"].Load(); got != 0 {
		t.Fatalf("response.failure = %d, want 0", got)
	}
	// A second response with the SAME corr must MISS (correlation erased) →
	// response.failure (AMEND-K4 unregistered-correlation arm).
	d.decodeOnWrite(buildResponse(42))
	if got := ks.counters["response.failure"].Load(); got != 1 {
		t.Fatalf("re-using an erased correlation must be a failure, got %d", got)
	}
	if got := ks.counters["response.metadata_response"].Load(); got != 1 {
		t.Fatalf("erased correlation must NOT re-count the response, got %d", got)
	}
}

func TestDecodeResponse_Unregistered(t *testing.T) {
	d, ks := newTestDecoder(t)
	resp := buildResponse(999) // never registered → response.failure
	d.decodeOnWrite(resp)
	if got := ks.counters["response.failure"].Load(); got != 1 {
		t.Fatalf("response.failure = %d, want 1 for unregistered correlation_id", got)
	}
}

func TestDecodeResponse_MalformedFrame(t *testing.T) {
	d, ks := newTestDecoder(t)
	// A COMPLETE response frame whose body is too short to hold a full 4-byte
	// correlation_id → within-frame short read on a fully-received frame →
	// response.failure (the codec.go decodeResponseFrame doc comment). Build the
	// raw beI32(N) + short-body pattern the request side uses (NOT buildResponse,
	// which builds a valid 4-byte-corr body).
	short := append(beI32(2), []byte{0x00, 0x12}...) // body len 2: correlation_id truncated
	d.decodeOnWrite(short)
	if got := ks.counters["response.failure"].Load(); got != 1 {
		t.Fatalf("short-body response frame must be a failure, got %d", got)
	}
	if got := ks.counters["response.unknown"].Load(); got != 0 {
		t.Fatalf("malformed frame must NOT count response.unknown, got %d", got)
	}
}

func TestDecodeResponse_UnknownKeyVersion(t *testing.T) {
	d, ks := newTestDecoder(t)
	// (a) register a corr against an UNKNOWN api_key → response.unknown.
	d.register(7, 9999, 0)
	d.decodeOnWrite(buildResponse(7))
	if got := ks.counters["response.unknown"].Load(); got != 1 {
		t.Fatalf("unknown api_key response = %d, want 1", got)
	}
	// (b) register a corr against a KNOWN key but a version ABOVE its max →
	// response.unknown. metadata(3) max is well below 0x7FFF.
	d.register(8, 3, 0x7FFF)
	d.decodeOnWrite(buildResponse(8))
	if got := ks.counters["response.unknown"].Load(); got != 2 {
		t.Fatalf("unknown-version response = %d, want 2", got)
	}
	if got := ks.counters["response.metadata_response"].Load(); got != 0 {
		t.Fatalf("unknown version must NOT count metadata_response, got %d", got)
	}
	if got := ks.counters["response.failure"].Load(); got != 0 {
		t.Fatalf("known-but-unknown classification must not be a failure, got %d", got)
	}
}

func TestDecodeResponse_PartialThenComplete(t *testing.T) {
	// The write seam is INCREMENTAL (writeChainConn.Write allocates a FRESH Buffer
	// per Write — ADR-0221), so decodeOnWrite appends each chunk directly and
	// reassembles across calls via writeBuf (mirroring mongoproxy).
	d, ks := newTestDecoder(t)
	req := buildRequest(3, 9, 77, "c", true) // metadata(3) v9 — registers corr 77
	d.decodeOnData(req, int64(len(req)))
	resp := buildResponse(77)
	// Feed first 3 bytes (less than the 4-byte length prefix), then the rest.
	d.decodeOnWrite(resp[:3])
	if got := ks.counters["response.metadata_response"].Load(); got != 0 {
		t.Fatalf("partial response frame must not count yet, got %d", got)
	}
	d.decodeOnWrite(resp[3:])
	if got := ks.counters["response.metadata_response"].Load(); got != 1 {
		t.Fatalf("completed response frame must count once, got %d", got)
	}
	if got := ks.counters["response.failure"].Load(); got != 0 {
		t.Fatalf("partial response frame must not be a failure, got %d", got)
	}
}

func TestCorrelation_Race(t *testing.T) {
	// Run with -race: a request goroutine registering + a response goroutine
	// recovering on the SAME decoder, asserting no data race on `expected`.
	d, _ := newTestDecoder(t)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		corr := int32(i)
		wg.Add(2)
		go func() { defer wg.Done(); d.register(corr, 3, 9) }()
		go func() { defer wg.Done(); d.lookupAndErase(corr) }()
	}
	wg.Wait()
}
