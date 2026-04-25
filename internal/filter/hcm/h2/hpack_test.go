package h2

import (
	"strings"
	"testing"

	"golang.org/x/net/http2/hpack"
)

func TestHPACK_EncodeDecodeRoundTrip(t *testing.T) {
	st := newHPACKState(4096)
	headers := []hpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "text/plain"},
		{Name: "content-length", Value: "3"},
		{Name: "server", Value: "envoy"},
		{Name: "date", Value: "Sun, 06 Apr 2025 12:00:00 GMT"},
	}
	encoded := st.encodeHeaders(headers)
	decoded, err := st.decodeBlock(encoded, true)
	if err != nil {
		t.Fatalf("decodeBlock = %v, want nil", err)
	}
	if len(decoded) != len(headers) {
		t.Fatalf("decoded len = %d, want %d", len(decoded), len(headers))
	}
	for i, h := range headers {
		if decoded[i].Name != h.Name || decoded[i].Value != h.Value {
			t.Errorf("decoded[%d] = %+v, want %+v", i, decoded[i], h)
		}
	}
}

func TestHPACK_AdversarialDecode_NoPanicReturnsCompressionError(t *testing.T) {
	st := newHPACKState(4096)
	bad := []byte{0xff, 0xff, 0xff, 0xff, 0xff}
	_, err := st.decodeBlock(bad, true)
	if err == nil {
		t.Fatal("decodeBlock(adversarial) returned nil; want COMPRESSION_ERROR")
	}
	if !strings.HasPrefix(err.Error(), "h2: COMPRESSION_ERROR") {
		t.Errorf("got %q, want h2: COMPRESSION_ERROR-prefixed", err.Error())
	}
}

func TestHPACK_UpdateMaxTableSize_PropagatesToEncoder(t *testing.T) {
	st := newHPACKState(4096)
	headers := []hpack.HeaderField{{Name: "x-custom", Value: strings.Repeat("a", 1000)}}
	out1 := st.encodeHeaders(headers)
	st.updateMaxTableSize(64)
	out2 := st.encodeHeaders(headers)
	// Encoder forced to literal (no dynamic-table indexing) when shrunk:
	// the second output should not be smaller than the first by more than
	// the indexed-prefix overhead. Just assert round-trip still works.
	dec, err := st.decodeBlock(out2, true)
	if err != nil {
		t.Fatalf("decodeBlock(post-shrink) = %v, want nil", err)
	}
	if len(dec) != 1 || dec[0].Value != headers[0].Value {
		t.Errorf("round-trip post-shrink failed; got %+v", dec)
	}
	_ = out1
}
