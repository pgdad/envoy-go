package driver

import (
	"encoding/hex"
	"errors"
	"testing"

	"golang.org/x/net/http2/hpack"
)

// TestGRPCFrame: the gRPC length-prefixed message framing is
// 1 byte compressed-flag (0) + 4-byte big-endian length + message. The two
// bodies the fixture sends are pinned by their exact hex.
func TestGRPCFrame(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  []byte
		want string
	}{
		// Check("") — the empty HealthCheckRequest (the success arm).
		{"empty", nil, "0000000000"},
		// Check("nope") — field 1, wire type 2, len 4, "nope" (notfound arm).
		{"nope", []byte{0x0A, 0x04, 'n', 'o', 'p', 'e'}, "000000000" + "6" + "0a046e6f7065"},
		// A 1-byte message exercises the low length byte on its own.
		{"one-byte", []byte{0xFF}, "00000000" + "01" + "ff"},
	} {
		got := hex.EncodeToString(grpcFrame(tc.msg))
		if got != tc.want {
			t.Errorf("grpcFrame(%s) = %s, want %s", tc.name, got, tc.want)
		}
		if want := 5 + len(tc.msg); len(grpcFrame(tc.msg)) != want {
			t.Errorf("grpcFrame(%s): length %d, want %d", tc.name, len(grpcFrame(tc.msg)), want)
		}
	}
}

// TestGRPCFrameBigLength: the 4-byte length is big-endian across byte
// boundaries (the encode helper builds it by hand).
func TestGRPCFrameBigLength(t *testing.T) {
	f := grpcFrame(make([]byte, 0x0102))
	if got, want := hex.EncodeToString(f[:5]), "0000000102"; got != want {
		t.Errorf("grpcFrame(258-byte msg) prefix = %s, want %s", got, want)
	}
}

// TestRequestFields: gRPC arms carry content-type + te after the four
// pseudo-headers; the plain arm carries neither. Order is the wire order.
func TestRequestFields(t *testing.T) {
	gotGRPC := formatFields(requestFields(arm{name: "success", meth: "POST", path: "/grpc.health.v1.Health/Check", grpc: true}), false)
	wantGRPC := ":method=POST :scheme=https :path=/grpc.health.v1.Health/Check :authority=localhost content-type=application/grpc te=trailers"
	if gotGRPC != wantGRPC {
		t.Errorf("requestFields(grpc):\n got %q\nwant %q", gotGRPC, wantGRPC)
	}

	gotPlain := formatFields(requestFields(arm{name: "plain", meth: "GET", path: "/plain", grpc: false}), false)
	wantPlain := ":method=GET :scheme=https :path=/plain :authority=localhost"
	if gotPlain != wantPlain {
		t.Errorf("requestFields(plain):\n got %q\nwant %q", gotPlain, wantPlain)
	}
}

// TestEncodeHeaderBlockRoundTrip: the hpack-encoded request block decodes back
// to the same fields, in the same order (the encoder must not reorder).
func TestEncodeHeaderBlockRoundTrip(t *testing.T) {
	in := requestFields(arm{name: "unimplemented", meth: "POST", path: "/grpc.health.v1.Health/Nope", grpc: true})
	block, err := encodeHeaderBlock(in)
	if err != nil {
		t.Fatalf("encodeHeaderBlock: unexpected error: %v", err)
	}
	out, err := hpack.NewDecoder(hpackTableSize, nil).DecodeFull(block)
	if err != nil {
		t.Fatalf("hpack decode: %v", err)
	}
	if len(out) != len(in) {
		t.Errorf("decoded %d fields, want %d", len(out), len(in))
		return
	}
	for i := range in {
		if out[i].Name != in[i].Name || out[i].Value != in[i].Value {
			t.Errorf("field %d: got %s=%s, want %s=%s", i, out[i].Name, out[i].Value, in[i].Name, in[i].Value)
		}
	}
}

// TestFormatFieldsFilter: the CLOSED canonicalization list drops exactly
// x-envoy-upstream-service-time and date, BY NAME, and nothing else — and the
// surviving fields stay in WIRE ORDER (no sort).
func TestFormatFieldsFilter(t *testing.T) {
	// Deliberately NOT in sorted order: a sort would reorder this input.
	fields := []hpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "application/grpc"},
		{Name: "x-envoy-upstream-service-time", Value: "3"},
		{Name: "trailer", Value: "Grpc-Status"},
		{Name: "date", Value: "Fri, 08 Aug 2026 00:00:00 GMT"},
		{Name: "server", Value: "envoy"},
	}

	got := formatFields(fields, true)
	want := ":status=200 content-type=application/grpc trailer=Grpc-Status server=envoy"
	if got != want {
		t.Errorf("formatFields(filter=true):\n got %q\nwant %q", got, want)
	}

	gotVerbatim := formatFields(fields, false)
	wantVerbatim := ":status=200 content-type=application/grpc x-envoy-upstream-service-time=3 trailer=Grpc-Status date=Fri, 08 Aug 2026 00:00:00 GMT server=envoy"
	if gotVerbatim != wantVerbatim {
		t.Errorf("formatFields(filter=false):\n got %q\nwant %q", gotVerbatim, wantVerbatim)
	}
}

// TestFormatFieldsTrailersVerbatim: trailers are rendered unfiltered and
// UNSORTED — grpc-go emits grpc-message BEFORE grpc-status and that wire order
// is part of what the fixture pins. A name that merely resembles a dropped one
// must survive (the drop is exact-name, not prefix).
func TestFormatFieldsTrailersVerbatim(t *testing.T) {
	trailers := []hpack.HeaderField{
		{Name: "grpc-message", Value: "unknown service grpc.health.v1.Nope"},
		{Name: "grpc-status", Value: "5"},
		{Name: "date-added", Value: "keepme"},
		{Name: "x-envoy-upstream-service-time-ish", Value: "keepme"},
	}
	got := formatFields(trailers, false)
	want := "grpc-message=unknown service grpc.health.v1.Nope grpc-status=5 date-added=keepme x-envoy-upstream-service-time-ish=keepme"
	if got != want {
		t.Errorf("formatFields(trailers):\n got %q\nwant %q", got, want)
	}
	// The same near-miss names survive even on the FILTERED (headers) leg.
	if gotF := formatFields(trailers, true); gotF != want {
		t.Errorf("formatFields(trailers, filter=true):\n got %q\nwant %q", gotF, want)
	}
}

// TestFormatFieldsEmpty: an empty field list renders as the empty string (the
// bracket pair in the transcript grammar stays, the contents do not).
func TestFormatFieldsEmpty(t *testing.T) {
	if got := formatFields(nil, true); got != "" {
		t.Errorf("formatFields(nil) = %q, want empty", got)
	}
}

// TestScrubAddr: the side-specific dial address is replaced everywhere it
// appears, so READ-ERR lines are cross-side comparable. Text without the addr
// passes through unchanged.
func TestScrubAddr(t *testing.T) {
	for _, tc := range []struct {
		in, addr, want string
	}{
		{"dial tcp 127.0.0.1:54321: connect: connection refused", "127.0.0.1:54321",
			"dial tcp <addr>: connect: connection refused"},
		{"read 127.0.0.1:1: x 127.0.0.1:1", "127.0.0.1:1", "read <addr>: x <addr>"},
		{"unexpected EOF", "127.0.0.1:9", "unexpected EOF"},
	} {
		if got := scrubAddr(tc.in, tc.addr); got != tc.want {
			t.Errorf("scrubAddr(%q, %q) = %q, want %q", tc.in, tc.addr, got, tc.want)
		}
	}
	if got := scrubAddr(errors.New("dial tcp 1.2.3.4:5: refused").Error(), "1.2.3.4:5"); got != "dial tcp <addr>: refused" {
		t.Errorf("scrubAddr on an error string = %q", got)
	}
}

// TestArms: the four arms, in transcript order, with the framing each one
// sends. The plain arm must be non-gRPC and bodyless (END_STREAM rides on its
// request HEADERS) — it is the trailer-less D-84-ENDSTREAM gate.
func TestArms(t *testing.T) {
	got := arms()
	want := []struct {
		name    string
		meth    string
		path    string
		grpc    bool
		bodyHex string // "" -> nil body
	}{
		{"success", "POST", "/grpc.health.v1.Health/Check", true, "0000000000"},
		{"notfound", "POST", "/grpc.health.v1.Health/Check", true, "00000000060a046e6f7065"},
		{"unimplemented", "POST", "/grpc.health.v1.Health/Nope", true, "0000000000"},
		{"plain", "GET", "/plain", false, ""},
	}
	if len(got) != len(want) {
		t.Fatalf("arms(): %d arms, want %d", len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.name != w.name || g.meth != w.meth || g.path != w.path || g.grpc != w.grpc {
			t.Errorf("arm %d: got {%s %s %s grpc=%t}, want {%s %s %s grpc=%t}",
				i, g.name, g.meth, g.path, g.grpc, w.name, w.meth, w.path, w.grpc)
		}
		if got := hex.EncodeToString(g.body); got != w.bodyHex {
			t.Errorf("arm %s: body hex %q, want %q", w.name, got, w.bodyHex)
		}
		if w.bodyHex == "" && g.body != nil {
			t.Errorf("arm %s: body must be nil so END_STREAM rides on request HEADERS", w.name)
		}
	}
}
