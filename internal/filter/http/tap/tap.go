package tap

import (
	"net/http"
	"strconv"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/headermatch"
)

// tapFilter is ONE value installed as BOTH HTTPFilter.Decoder and
// HTTPFilter.Encoder. FilterChain.Destroy()'s loop is
// `if Decoder != nil {…} else if Encoder != nil {…}`, so an encoder-only
// OnDestroy is UNREACHABLE and the stream-end emit would silently never fire.
// See doc.go.
type tapFilter struct {
	cfg *config

	decCB envoyhttp.DecoderFilterCallbacks
	encCB envoyhttp.EncoderFilterCallbacks

	// Lowercase-keyed COPIES; used for BOTH matching and emission.
	reqHdrs  http.Header
	respHdrs http.Header
	sawReq   bool
	sawResp  bool
}

func (f *tapFilter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.decCB = cb }
func (f *tapFilter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.encCB = cb }

// DecodeHeaders captures a lowercased COPY of the request headers. It never
// emits: a tap trace is an end-of-stream artifact (AMEND-TAP-NOEARLYEMIT).
func (f *tapFilter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	f.reqHdrs = headermatch.Lowercase(headers)
	f.sawReq = true
	return envoyhttp.Continue
}

// EncodeHeaders captures a lowercased COPY of the response headers and injects
// the synthetic :status from the ADR-0196 accessor INTO THE COPY.
//
// NEVER write into `headers`: HCM merges that very map back into the response
// it writes to the socket (connection.go:738 -> :741 -> writeH1Reply), and
// textproto canonicalization does not strip a leading colon, so a synthetic
// ":status" would be emitted as a literal header on the wire.
func (f *tapFilter) EncodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	lc := headermatch.Lowercase(headers)
	if f.encCB != nil {
		if st := f.encCB.ResponseStatus(); st > 0 {
			lc[":status"] = []string{strconv.Itoa(st)}
		}
	}
	f.respHdrs = lc
	f.sawResp = true
	return envoyhttp.Continue
}

// Tap observes headers only; the data and trailer hooks are inert pass-throughs.
func (f *tapFilter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}
func (f *tapFilter) EncodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}
func (f *tapFilter) DecodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
func (f *tapFilter) EncodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
