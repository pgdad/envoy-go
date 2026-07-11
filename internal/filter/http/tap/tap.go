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

	// 56.2 body capture: filter-owned accumulation (buffer/buffer.go SHAPE).
	reqBody     []byte
	reqTrunc    bool
	respBody    []byte
	respTrunc   bool
	sawReqBody  bool // hook fired at least once (distinct from len(reqBody)==0)
	sawRespBody bool
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

// accumulate appends chunk to buf, honoring the byte limit. Truncation is
// strict `>` (AMEND-TAP-BODY2-BOUNDARY): a body EXACTLY at the limit is NOT
// truncated. Once the limit is reached *trunc is set and no further bytes are
// appended. Returns the (possibly-grown) buffer.
func accumulate(buf []byte, trunc *bool, chunk []byte, limit uint32) []byte {
	room := int(limit) - len(buf)
	if len(chunk) > room { // strict >: len(chunk) fits iff len(chunk) <= room
		if room > 0 {
			buf = append(buf, chunk[:room]...)
		}
		*trunc = true
		return buf
	}
	return append(buf, chunk...)
}

// DecodeData accumulates the request body up to cfg.maxRx. Tap is a READ-ONLY
// observer: it returns DataContinue ALWAYS (never StopIteration/OverwriteBody)
// so the body flows unperturbed to the upstream. endStream is accepted for
// signature conformance; the buffer IS the state, no finalization is needed.
func (f *tapFilter) DecodeData(data []byte, _ bool) envoyhttp.FilterDataStatus {
	f.sawReqBody = true
	f.reqBody = accumulate(f.reqBody, &f.reqTrunc, data, f.cfg.maxRx)
	return envoyhttp.DataContinue
}

// EncodeData is symmetric into respBody up to cfg.maxTx.
func (f *tapFilter) EncodeData(data []byte, _ bool) envoyhttp.FilterDataStatus {
	f.sawRespBody = true
	f.respBody = accumulate(f.respBody, &f.respTrunc, data, f.cfg.maxTx)
	return envoyhttp.DataContinue
}
func (f *tapFilter) DecodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
func (f *tapFilter) EncodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
