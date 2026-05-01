package http

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// recordingFilter logs each callback for assertion.
type recordingFilter struct {
	name           string
	headersStatus  FilterHeadersStatus
	dataStatus     FilterDataStatus
	trailersStatus FilterTrailersStatus
	decodeHeaders  atomic.Int32
	decodeData     atomic.Int32
	decodeTrailers atomic.Int32
	encodeHeaders  atomic.Int32
	encodeData     atomic.Int32
	encodeTrailers atomic.Int32
	destroyed      atomic.Int32
	dcb            DecoderFilterCallbacks
	ecb            EncoderFilterCallbacks
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
	return Continue
}
func (f *recordingFilter) EncodeData([]byte, bool) FilterDataStatus {
	f.encodeData.Add(1)
	return DataContinue
}
func (f *recordingFilter) EncodeTrailers(http.Header) FilterTrailersStatus {
	f.encodeTrailers.Add(1)
	return TrailersContinue
}
func (f *recordingFilter) SetEncoderCallbacks(cb EncoderFilterCallbacks) { f.ecb = cb }
func (f *recordingFilter) OnDestroy()                                     { f.destroyed.Add(1) }

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
