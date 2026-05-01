package http

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recordingFilter logs each callback for assertion.
type recordingFilter struct {
	name string
	// Decode-side return statuses.
	headersStatus  FilterHeadersStatus
	dataStatus     FilterDataStatus
	trailersStatus FilterTrailersStatus
	// Encode-side return statuses (separate so encode/decode can diverge in
	// tests without coupling — added for Task 6).
	encHeadersStatus  FilterHeadersStatus
	encDataStatus     FilterDataStatus
	encTrailersStatus FilterTrailersStatus
	decodeHeaders     atomic.Int32
	decodeData        atomic.Int32
	decodeTrailers    atomic.Int32
	encodeHeaders     atomic.Int32
	encodeData        atomic.Int32
	encodeTrailers    atomic.Int32
	destroyed         atomic.Int32
	dcb               DecoderFilterCallbacks
	ecb               EncoderFilterCallbacks
}

func (f *recordingFilter) DecodeHeaders(http.Header, bool) FilterHeadersStatus {
	f.decodeHeaders.Add(1)
	return f.headersStatus
}
func (f *recordingFilter) DecodeData([]byte, bool) FilterDataStatus {
	f.decodeData.Add(1)
	return f.dataStatus
}
func (f *recordingFilter) DecodeTrailers(http.Header) FilterTrailersStatus {
	f.decodeTrailers.Add(1)
	return f.trailersStatus
}
func (f *recordingFilter) SetDecoderCallbacks(cb DecoderFilterCallbacks) { f.dcb = cb }
func (f *recordingFilter) EncodeHeaders(http.Header, bool) FilterHeadersStatus {
	f.encodeHeaders.Add(1)
	return f.encHeadersStatus
}
func (f *recordingFilter) EncodeData([]byte, bool) FilterDataStatus {
	f.encodeData.Add(1)
	return f.encDataStatus
}
func (f *recordingFilter) EncodeTrailers(http.Header) FilterTrailersStatus {
	f.encodeTrailers.Add(1)
	return f.encTrailersStatus
}
func (f *recordingFilter) SetEncoderCallbacks(cb EncoderFilterCallbacks) { f.ecb = cb }
func (f *recordingFilter) OnDestroy()                                    { f.destroyed.Add(1) }

func newChainOf(filters ...*recordingFilter) (*FilterChain, []*recordingFilter) {
	hf := make([]HTTPFilter, len(filters))
	for i, f := range filters {
		hf[i] = HTTPFilter{Name: f.name, Decoder: f, Encoder: f}
	}
	return NewFilterChain(hf, nil), filters
}

func TestChain_Decode_AllContinue(t *testing.T) {
	a := &recordingFilter{name: "a", headersStatus: Continue, dataStatus: DataContinue, trailersStatus: TrailersContinue}
	b := &recordingFilter{name: "b", headersStatus: Continue, dataStatus: DataContinue, trailersStatus: TrailersContinue}
	chain, _ := newChainOf(a, b)
	terminated, err := chain.RunDecodeHeaders(context.Background(), http.Header{}, true)
	if err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("expected RunDecodeHeaders to report iteration-complete (terminated=true)")
	}
	if a.decodeHeaders.Load() != 1 || b.decodeHeaders.Load() != 1 {
		t.Fatalf("expected each filter's DecodeHeaders called once; got a=%d b=%d", a.decodeHeaders.Load(), b.decodeHeaders.Load())
	}
}

func TestChain_Decode_StopIteration_ResumeAdvances(t *testing.T) {
	a := &recordingFilter{name: "a", headersStatus: StopIteration, dataStatus: DataContinue, trailersStatus: TrailersContinue}
	b := &recordingFilter{name: "b", headersStatus: Continue, dataStatus: DataContinue, trailersStatus: TrailersContinue}
	chain, _ := newChainOf(a, b)
	go func() {
		time.Sleep(20 * time.Millisecond)
		a.dcb.ContinueDecoding()
	}()
	terminated, err := chain.RunDecodeHeaders(context.Background(), http.Header{}, true)
	if err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("expected iteration to complete after async resume")
	}
	if a.decodeHeaders.Load() != 1 || b.decodeHeaders.Load() != 1 {
		t.Fatalf("expected b's DecodeHeaders to run after resume; a=%d b=%d", a.decodeHeaders.Load(), b.decodeHeaders.Load())
	}
}

func TestChain_Decode_StopIteration_CtxCancelAborts(t *testing.T) {
	a := &recordingFilter{name: "a", headersStatus: StopIteration}
	b := &recordingFilter{name: "b", headersStatus: Continue}
	chain, _ := newChainOf(a, b)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	terminated, err := chain.RunDecodeHeaders(ctx, http.Header{}, true)
	if err == nil {
		t.Fatalf("expected ctx-cancel error")
	}
	if terminated {
		t.Fatalf("expected aborted iteration; got terminated=true")
	}
	chain.Destroy()
	if a.destroyed.Load() == 0 || b.destroyed.Load() == 0 {
		t.Fatalf("expected OnDestroy to fire on chain.Destroy after ctx-cancel; a=%d b=%d", a.destroyed.Load(), b.destroyed.Load())
	}
}

// encodeRecorder wraps a recordingFilter as a StreamEncoderFilter so that
// per-filter EncodeHeaders invocation order can be observed independent of
// the underlying recording counters. Used by TestChain_Encode_ReverseOrder
// to assert the SPEC §11.1 empirical pin.
type encodeRecorder struct {
	f     *recordingFilter
	order *[]string
	mu    *sync.Mutex
}

func (e encodeRecorder) EncodeHeaders(h http.Header, end bool) FilterHeadersStatus {
	e.mu.Lock()
	*e.order = append(*e.order, e.f.name)
	e.mu.Unlock()
	return e.f.EncodeHeaders(h, end)
}
func (e encodeRecorder) EncodeData(d []byte, end bool) FilterDataStatus      { return e.f.EncodeData(d, end) }
func (e encodeRecorder) EncodeTrailers(t http.Header) FilterTrailersStatus   { return e.f.EncodeTrailers(t) }
func (e encodeRecorder) SetEncoderCallbacks(cb EncoderFilterCallbacks)       { e.f.SetEncoderCallbacks(cb) }
func (e encodeRecorder) OnDestroy()                                          { e.f.OnDestroy() }

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestChain_Encode_ReverseOrder(t *testing.T) {
	order := make([]string, 0, 3)
	var orderMu sync.Mutex
	mk := func(name string) *recordingFilter {
		return &recordingFilter{
			name:              name,
			headersStatus:     Continue,
			dataStatus:        DataContinue,
			trailersStatus:    TrailersContinue,
			encHeadersStatus:  Continue,
			encDataStatus:     DataContinue,
			encTrailersStatus: TrailersContinue,
		}
	}
	a := mk("a")
	b := mk("b")
	c := mk("c")
	hf := []HTTPFilter{
		{Name: "a", Decoder: a, Encoder: encodeRecorder{f: a, order: &order, mu: &orderMu}},
		{Name: "b", Decoder: b, Encoder: encodeRecorder{f: b, order: &order, mu: &orderMu}},
		{Name: "c", Decoder: c, Encoder: encodeRecorder{f: c, order: &order, mu: &orderMu}},
	}
	chain := NewFilterChain(hf, nil)
	terminated, err := chain.RunEncodeHeaders(context.Background(), http.Header{}, true)
	if err != nil {
		t.Fatalf("RunEncodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("expected iteration complete")
	}
	want := []string{"c", "b", "a"}
	if !equalSlice(order, want) {
		t.Fatalf("expected encode order %v; got %v", want, order)
	}
}

func TestChain_Encode_StopIteration_ResumeAdvances(t *testing.T) {
	// b returns StopIteration on encode; iteration starts at index len-1=1 (b),
	// parks, then async ContinueEncoding unparks; cursor advances to a (index 0)
	// which returns Continue and completes.
	a := &recordingFilter{
		name:              "a",
		encHeadersStatus:  Continue,
		encDataStatus:     DataContinue,
		encTrailersStatus: TrailersContinue,
	}
	b := &recordingFilter{
		name:              "b",
		encHeadersStatus:  StopIteration,
		encDataStatus:     DataContinue,
		encTrailersStatus: TrailersContinue,
	}
	chain, _ := newChainOf(a, b)
	go func() {
		time.Sleep(20 * time.Millisecond)
		b.ecb.ContinueEncoding()
	}()
	terminated, err := chain.RunEncodeHeaders(context.Background(), http.Header{}, true)
	if err != nil {
		t.Fatalf("RunEncodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("expected iteration to complete after async resume")
	}
	if a.encodeHeaders.Load() != 1 || b.encodeHeaders.Load() != 1 {
		t.Fatalf("expected each filter's EncodeHeaders called once; got a=%d b=%d", a.encodeHeaders.Load(), b.encodeHeaders.Load())
	}
}

func TestChain_Encode_StopIteration_CtxCancelAborts(t *testing.T) {
	// Encode-side analogue of TestChain_Decode_StopIteration_CtxCancelAborts.
	// b returns StopIteration on encode; ctx cancellation during park yields
	// ctx.Err and terminated=false.
	a := &recordingFilter{name: "a", encHeadersStatus: Continue}
	b := &recordingFilter{name: "b", encHeadersStatus: StopIteration}
	chain, _ := newChainOf(a, b)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	terminated, err := chain.RunEncodeHeaders(ctx, http.Header{}, true)
	if err == nil {
		t.Fatalf("expected ctx-cancel error")
	}
	if terminated {
		t.Fatalf("expected aborted iteration; got terminated=true")
	}
	chain.Destroy()
	if a.destroyed.Load() == 0 || b.destroyed.Load() == 0 {
		t.Fatalf("expected OnDestroy to fire on chain.Destroy after ctx-cancel; a=%d b=%d", a.destroyed.Load(), b.destroyed.Load())
	}
}
