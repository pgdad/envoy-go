package tap

import (
	"bytes"
	"net/http"
	"reflect"
	"testing"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// stubEncCB satisfies EncoderFilterCallbacks by embedding the interface: only
// ResponseStatus is implemented. Any other method call nil-panics, which is the
// point — tap must not reach for anything else on the encode side.
type stubEncCB struct {
	envoyhttp.EncoderFilterCallbacks
	status int
}

func (s stubEncCB) ResponseStatus() int { return s.status }

func newTapFilter(t *testing.T) *tapFilter {
	t.Helper()
	ctx, _ := newCtx()
	factory, err := New(mustAny(t, validTap(t.TempDir())), ctx)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hf := factory()
	f, ok := hf.Decoder.(*tapFilter)
	if !ok {
		t.Fatalf("Decoder is %T, want *tapFilter", hf.Decoder)
	}
	return f
}

// THE WIRE-LEAK REGRESSION. The map handed to EncodeHeaders is merged back into
// the wire response; a synthetic :status added to it would be emitted literally.
func TestEncodeHeaders_NeverMutatesTheWireBoundMap(t *testing.T) {
	f := newTapFilter(t)
	f.SetEncoderCallbacks(stubEncCB{status: 204})

	wire := http.Header{"Content-Type": {"text/plain"}}
	before := wire.Clone()

	if got := f.EncodeHeaders(wire, true); got != envoyhttp.Continue {
		t.Errorf("EncodeHeaders = %v, want Continue", got)
	}
	if !reflect.DeepEqual(wire, before) {
		t.Errorf("EncodeHeaders MUTATED the wire-bound map:\n got %v\nwant %v", wire, before)
	}
	if _, leaked := wire[":status"]; leaked {
		t.Errorf(":status leaked into the wire-bound header map")
	}
	// ...but the captured copy DOES carry it, lowercased.
	if got := f.respHdrs[":status"]; len(got) != 1 || got[0] != "204" {
		t.Errorf("captured response headers :status = %v, want [204]", got)
	}
	//nolint:staticcheck // SA1008: f.respHdrs is a lowercase-keyed COPY (headermatch.Lowercase), not a canonical http.Header.
	if got := f.respHdrs["content-type"]; len(got) != 1 || got[0] != "text/plain" {
		t.Errorf("captured response headers content-type = %v, want [text/plain]", got)
	}
}

func TestDecodeHeaders_LowercasesAndCopies(t *testing.T) {
	f := newTapFilter(t)
	req := http.Header{"X-Tap": {"yes"}, ":method": {"GET"}, ":path": {"/tap"}}
	before := req.Clone()

	if got := f.DecodeHeaders(req, true); got != envoyhttp.Continue {
		t.Errorf("DecodeHeaders = %v, want Continue", got)
	}
	if !reflect.DeepEqual(req, before) {
		t.Errorf("DecodeHeaders MUTATED its input map")
	}
	//nolint:staticcheck // SA1008: f.reqHdrs is a lowercase-keyed COPY (headermatch.Lowercase), not a canonical http.Header.
	if got := f.reqHdrs["x-tap"]; len(got) != 1 || got[0] != "yes" {
		t.Errorf("captured x-tap = %v, want [yes]", got)
	}
	if got := f.reqHdrs[":method"]; len(got) != 1 || got[0] != "GET" {
		t.Errorf("captured :method = %v, want [GET]", got)
	}
}

func TestToHeaderValues_SortedByKeyThenValue_RawValueNil(t *testing.T) {
	h := http.Header{"b": {"2"}, "a": {"z", "y"}, ":status": {"204"}}
	got := toHeaderValues(h)
	type kv struct{ k, v string }
	var flat []kv
	for _, hv := range got {
		if hv.GetRawValue() != nil {
			t.Errorf("RawValue must be nil (protojson EmitDefaultValues renders \"\"); got %v", hv.GetRawValue())
		}
		flat = append(flat, kv{hv.GetKey(), hv.GetValue()})
	}
	want := []kv{{":status", "204"}, {"a", "y"}, {"a", "z"}, {"b", "2"}}
	if !reflect.DeepEqual(flat, want) {
		t.Errorf("toHeaderValues = %v, want %v", flat, want)
	}
}

// A missing/zero ResponseStatus must not synthesize a bogus :status.
func TestEncodeHeaders_NoStatusWhenCallbacksAbsent(t *testing.T) {
	f := newTapFilter(t)
	f.EncodeHeaders(http.Header{"Content-Type": {"text/plain"}}, true)
	if _, ok := f.respHdrs[":status"]; ok {
		t.Errorf(":status must be absent when no encoder callbacks are set")
	}
}

func newBodyFilter(maxRx, maxTx uint32) *tapFilter {
	return &tapFilter{cfg: &config{maxRx: maxRx, maxTx: maxTx}}
}

func TestDecodeData_SingleChunkUnderCap(t *testing.T) {
	f := newBodyFilter(20, 20)
	if st := f.DecodeData([]byte("0123456789"), true); st != envoyhttp.DataContinue {
		t.Errorf("DecodeData status = %v, want DataContinue (tap is read-only)", st)
	}
	if string(f.reqBody) != "0123456789" {
		t.Errorf("reqBody = %q, want %q", f.reqBody, "0123456789")
	}
	if f.reqTrunc {
		t.Errorf("reqTrunc = true, want false (10 < cap 20)")
	}
	if !f.sawReqBody {
		t.Errorf("sawReqBody = false, want true (hook fired)")
	}
}

func TestDecodeData_MultiChunkAccumulates(t *testing.T) {
	f := newBodyFilter(100, 100)
	f.DecodeData([]byte("AAA"), false)
	f.DecodeData([]byte("BBB"), false)
	f.DecodeData(nil, true) // synthetic end (connection.go:653)
	if string(f.reqBody) != "AAABBB" {
		t.Errorf("reqBody = %q, want %q (chunks must concatenate)", f.reqBody, "AAABBB")
	}
	if f.reqTrunc {
		t.Errorf("reqTrunc = true, want false")
	}
}

func TestDecodeData_Over32KiBAccumulates(t *testing.T) {
	f := newBodyFilter(1<<20, 1<<20) // 1 MiB cap, above 32 KiB
	chunk := bytes.Repeat([]byte("x"), 32*1024)
	f.DecodeData(chunk, false)
	f.DecodeData(chunk, true) // 64 KiB total across two real-sized chunks
	if len(f.reqBody) != 64*1024 {
		t.Errorf("reqBody len = %d, want %d (must accumulate across >32KiB)", len(f.reqBody), 64*1024)
	}
	if f.reqTrunc {
		t.Errorf("reqTrunc = true, want false (64KiB < 1MiB cap)")
	}
}

func TestDecodeData_AtCapNotTruncated(t *testing.T) { // strict-> boundary
	f := newBodyFilter(10, 10)
	f.DecodeData([]byte("0123456789"), true) // exactly 10 == cap
	if string(f.reqBody) != "0123456789" {
		t.Errorf("reqBody = %q, want full 10 bytes", f.reqBody)
	}
	if f.reqTrunc {
		t.Errorf("reqTrunc = true, want FALSE (body length == cap is NOT truncated; strict >)")
	}
}

func TestDecodeData_OverCapTruncates(t *testing.T) {
	f := newBodyFilter(10, 10)
	f.DecodeData([]byte("0123456789ABCDEF"), true) // 16 > cap 10
	if string(f.reqBody) != "0123456789" {
		t.Errorf("reqBody = %q, want first 10 bytes only", f.reqBody)
	}
	if !f.reqTrunc {
		t.Errorf("reqTrunc = false, want true (16 > cap 10)")
	}
}

func TestDecodeData_ChunkStraddlesCap(t *testing.T) {
	f := newBodyFilter(10, 10)
	f.DecodeData([]byte("012345"), false)  // 6 bytes, under cap
	f.DecodeData([]byte("6789ABCD"), true) // 6+8=14 > 10: append prefix "6789" (4 = 10-6)
	if string(f.reqBody) != "0123456789" {
		t.Errorf("reqBody = %q, want %q (only cap-capturedLen prefix of the straddling chunk)", f.reqBody, "0123456789")
	}
	if !f.reqTrunc {
		t.Errorf("reqTrunc = false, want true (14 > cap 10)")
	}
}

func TestDecodeData_CapZeroNonEmpty_EmptyButTruncated(t *testing.T) {
	f := newBodyFilter(0, 0)
	f.DecodeData([]byte("nonempty"), true)
	if len(f.reqBody) != 0 {
		t.Errorf("reqBody len = %d, want 0 (cap 0 captures nothing)", len(f.reqBody))
	}
	if !f.reqTrunc {
		t.Errorf("reqTrunc = false, want true (cap 0 + non-empty body -> truncated)")
	}
	if !f.sawReqBody {
		t.Errorf("sawReqBody = false, want true (hook fired -> body must be PRESENT even when empty)")
	}
}

func TestEncodeData_SymmetricIntoRespBody(t *testing.T) {
	f := newBodyFilter(20, 5)
	if st := f.EncodeData([]byte("HELLOWORLD"), true); st != envoyhttp.DataContinue {
		t.Errorf("EncodeData status = %v, want DataContinue", st)
	}
	if string(f.respBody) != "HELLO" {
		t.Errorf("respBody = %q, want %q (cap 5)", f.respBody, "HELLO")
	}
	if !f.respTrunc {
		t.Errorf("respTrunc = false, want true (10 > cap 5)")
	}
	if !f.sawRespBody {
		t.Errorf("sawRespBody = false, want true")
	}
}
