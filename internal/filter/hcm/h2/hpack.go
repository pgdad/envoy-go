package h2

import (
	"bytes"

	"golang.org/x/net/http2/hpack"
)

// hpackState carries per-connection HPACK encoder + decoder state. Phase 05.1
// uses it as a single-threaded helper (one per ServerConn); no internal mutex.
// Per ADR-0046, hpack.Encoder and hpack.Decoder are the low-level codec
// surfaces; envoy-go owns the table-size update propagation across SETTINGS.
type hpackState struct {
	enc    *hpack.Encoder
	encBuf bytes.Buffer
	dec    *hpack.Decoder
	fields []hpack.HeaderField
}

// newHPACKState constructs encoder + decoder both initialized with maxTableSize
// (defaults to 4096 per ADR-0047). The decoder's emit-callback appends into
// the fields slice; decodeBlock resets and returns it on each call.
func newHPACKState(maxTableSize uint32) *hpackState {
	st := &hpackState{}
	st.enc = hpack.NewEncoder(&st.encBuf)
	st.enc.SetMaxDynamicTableSize(maxTableSize)
	st.dec = hpack.NewDecoder(maxTableSize, func(f hpack.HeaderField) {
		st.fields = append(st.fields, f)
	})
	return st
}

// encodeHeaders writes each header through the encoder, returning the encoded
// byte block. The returned bytes alias the internal buffer; callers must copy
// before storing across encode calls.
func (s *hpackState) encodeHeaders(headers []hpack.HeaderField) []byte {
	s.encBuf.Reset()
	for _, h := range headers {
		_ = s.enc.WriteField(h)
	}
	return s.encBuf.Bytes()
}

// decodeBlock feeds block into the decoder. If endBlock is true, it calls
// dec.Close() (signaling end of header block boundary). Returns the decoded
// fields (a fresh slice) or *Error{Code: COMPRESSION_ERROR} on adversarial
// input.
func (s *hpackState) decodeBlock(block []byte, endBlock bool) ([]hpack.HeaderField, error) {
	s.fields = s.fields[:0]
	if _, err := s.dec.Write(block); err != nil {
		return nil, &Error{Code: ErrCompressionError, Msg: "hpack decode", Underlying: err}
	}
	if endBlock {
		if err := s.dec.Close(); err != nil {
			return nil, &Error{Code: ErrCompressionError, Msg: "hpack close", Underlying: err}
		}
	}
	out := make([]hpack.HeaderField, len(s.fields))
	copy(out, s.fields)
	return out, nil
}

// updateMaxTableSize propagates a peer-announced SETTINGS_HEADER_TABLE_SIZE
// change to our encoder so subsequent outgoing HEADERS frames respect the
// peer's new limit. The decoder side is bounded by our advertised value
// (newHPACKState's argument); we do NOT re-advertise after handshake in 05.1.
func (s *hpackState) updateMaxTableSize(size uint32) {
	s.enc.SetMaxDynamicTableSize(size)
}
