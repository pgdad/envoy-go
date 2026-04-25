package h2

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorCodeStrings(t *testing.T) {
	cases := map[ErrCode]string{
		ErrNoError:            "NO_ERROR",
		ErrProtocolError:      "PROTOCOL_ERROR",
		ErrInternalError:      "INTERNAL_ERROR",
		ErrFlowControlError:   "FLOW_CONTROL_ERROR",
		ErrSettingsTimeout:    "SETTINGS_TIMEOUT",
		ErrStreamClosed:       "STREAM_CLOSED",
		ErrFrameSizeError:     "FRAME_SIZE_ERROR",
		ErrRefusedStream:      "REFUSED_STREAM",
		ErrCancel:             "CANCEL",
		ErrCompressionError:   "COMPRESSION_ERROR",
		ErrConnectError:       "CONNECT_ERROR",
		ErrEnhanceYourCalm:    "ENHANCE_YOUR_CALM",
		ErrInadequateSecurity: "INADEQUATE_SECURITY",
		ErrHTTP11Required:     "HTTP_1_1_REQUIRED",
	}
	for code, want := range cases {
		got := code.String()
		if got != want {
			t.Errorf("ErrCode(%d).String() = %q, want %q", uint32(code), got, want)
		}
	}
}

func TestConnError_PrefixAndShape(t *testing.T) {
	e := connError(ErrProtocolError, "bad preface")
	if !strings.HasPrefix(e.Error(), "h2: ") {
		t.Errorf("connError().Error() = %q, want h2:-prefixed", e.Error())
	}
	if got := e.Code; got != ErrProtocolError {
		t.Errorf("Code = %v, want PROTOCOL_ERROR", got)
	}
	if got := e.Stream; got != 0 {
		t.Errorf("Stream = %d, want 0 (conn-scoped)", got)
	}
}

func TestStreamError_PrefixAndShape(t *testing.T) {
	e := streamError(ErrInternalError, 5, "router action on h2")
	if !strings.HasPrefix(e.Error(), "h2: ") {
		t.Errorf("streamError().Error() = %q, want h2:-prefixed", e.Error())
	}
	if !strings.Contains(e.Error(), "stream=5") {
		t.Errorf("streamError().Error() = %q, want substring 'stream=5'", e.Error())
	}
	if got := e.Stream; got != 5 {
		t.Errorf("Stream = %d, want 5", got)
	}
}

func TestError_UnwrapsUnderlying(t *testing.T) {
	inner := errors.New("inner cause")
	e := &Error{Code: ErrInternalError, Underlying: inner}
	if got := errors.Unwrap(e); got != inner {
		t.Errorf("Unwrap = %v, want %v", got, inner)
	}
}
