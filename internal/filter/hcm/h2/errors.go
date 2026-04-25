package h2

import "fmt"

// ErrCode mirrors the RFC 9113 §7 error code numeric assignments. The
// String method returns the RFC 9113 mnemonic ("PROTOCOL_ERROR" etc.) used
// in error messages and (indirectly) in fuzz test assertions.
type ErrCode uint32

const (
	ErrNoError            ErrCode = 0x0
	ErrProtocolError      ErrCode = 0x1
	ErrInternalError      ErrCode = 0x2
	ErrFlowControlError   ErrCode = 0x3
	ErrSettingsTimeout    ErrCode = 0x4
	ErrStreamClosed       ErrCode = 0x5
	ErrFrameSizeError     ErrCode = 0x6
	ErrRefusedStream      ErrCode = 0x7
	ErrCancel             ErrCode = 0x8
	ErrCompressionError   ErrCode = 0x9
	ErrConnectError       ErrCode = 0xa
	ErrEnhanceYourCalm    ErrCode = 0xb
	ErrInadequateSecurity ErrCode = 0xc
	ErrHTTP11Required     ErrCode = 0xd
)

func (c ErrCode) String() string {
	switch c {
	case ErrNoError:
		return "NO_ERROR"
	case ErrProtocolError:
		return "PROTOCOL_ERROR"
	case ErrInternalError:
		return "INTERNAL_ERROR"
	case ErrFlowControlError:
		return "FLOW_CONTROL_ERROR"
	case ErrSettingsTimeout:
		return "SETTINGS_TIMEOUT"
	case ErrStreamClosed:
		return "STREAM_CLOSED"
	case ErrFrameSizeError:
		return "FRAME_SIZE_ERROR"
	case ErrRefusedStream:
		return "REFUSED_STREAM"
	case ErrCancel:
		return "CANCEL"
	case ErrCompressionError:
		return "COMPRESSION_ERROR"
	case ErrConnectError:
		return "CONNECT_ERROR"
	case ErrEnhanceYourCalm:
		return "ENHANCE_YOUR_CALM"
	case ErrInadequateSecurity:
		return "INADEQUATE_SECURITY"
	case ErrHTTP11Required:
		return "HTTP_1_1_REQUIRED"
	default:
		return fmt.Sprintf("UNKNOWN_ERR_CODE(0x%x)", uint32(c))
	}
}

// Error carries an RFC 9113 error code plus optional stream-id (0 means
// connection-scoped) plus an optional underlying error. Error strings start
// with "h2: " — the discriminator the fuzz targets check.
type Error struct {
	Code       ErrCode
	Stream     uint32 // 0 = connection-scoped
	Msg        string
	Underlying error
}

func (e *Error) Error() string {
	var prefix string
	if e.Stream == 0 {
		prefix = fmt.Sprintf("h2: %s", e.Code)
	} else {
		prefix = fmt.Sprintf("h2: %s stream=%d", e.Code, e.Stream)
	}
	if e.Msg != "" {
		prefix += ": " + e.Msg
	}
	if e.Underlying != nil {
		prefix += ": " + e.Underlying.Error()
	}
	return prefix
}

func (e *Error) Unwrap() error { return e.Underlying }

func connError(code ErrCode, msg string) *Error {
	return &Error{Code: code, Msg: msg}
}

func streamError(code ErrCode, stream uint32, msg string) *Error {
	return &Error{Code: code, Stream: stream, Msg: msg}
}
