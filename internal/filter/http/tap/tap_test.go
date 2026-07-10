package tap

import (
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
