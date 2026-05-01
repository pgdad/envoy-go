package http

import (
	"net/http"
	"testing"
)

func TestFilterHeadersStatus_Values(t *testing.T) {
	if Continue == StopIteration {
		t.Fatalf("Continue and StopIteration must differ")
	}
	if int(Continue) != 0 || int(StopIteration) != 1 {
		t.Fatalf("expected Continue=0, StopIteration=1; got Continue=%d, StopIteration=%d", Continue, StopIteration)
	}
}

func TestFilterDataStatus_Values(t *testing.T) {
	if DataContinue == DataStopIterationAndBuffer || DataStopIterationAndBuffer == DataStopIterationNoBuffer {
		t.Fatalf("data status enums must be distinct")
	}
}

func TestFilterTrailersStatus_Values(t *testing.T) {
	if TrailersContinue == TrailersStopIteration {
		t.Fatalf("trailers status enums must be distinct")
	}
}

// Compile-only assertion: a test-only fake filter implements both decoder and
// encoder interfaces with the expected method set. If any method is renamed,
// this test fails to compile.
type fakeFilter struct{}

func (fakeFilter) DecodeHeaders(http.Header, bool) FilterHeadersStatus    { return Continue }
func (fakeFilter) DecodeData([]byte, bool) FilterDataStatus               { return DataContinue }
func (fakeFilter) DecodeTrailers(http.Header) FilterTrailersStatus        { return TrailersContinue }
func (fakeFilter) SetDecoderCallbacks(DecoderFilterCallbacks)             {}
func (fakeFilter) EncodeHeaders(http.Header, bool) FilterHeadersStatus    { return Continue }
func (fakeFilter) EncodeData([]byte, bool) FilterDataStatus               { return DataContinue }
func (fakeFilter) EncodeTrailers(http.Header) FilterTrailersStatus        { return TrailersContinue }
func (fakeFilter) SetEncoderCallbacks(EncoderFilterCallbacks)             {}
func (fakeFilter) OnDestroy()                                             {}

func TestFilterInterfaces_Compile(t *testing.T) {
	var _ StreamDecoderFilter = fakeFilter{}
	var _ StreamEncoderFilter = fakeFilter{}
}
