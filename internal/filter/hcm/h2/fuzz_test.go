package h2

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2/hpack"
)

// FuzzFrameStream mutates a corpus of well-formed frame sequences and asserts
// no panic + every returned error begins with "h2:" (or is a context error).
//
// Per ADR-0018: 30-second short-budget in CI. Three seed entries:
//  1. Client preface only.
//  2. Preface + minimal SETTINGS frame (9-byte header, 0-byte payload).
//  3. Preface + SETTINGS + SETTINGS ACK (common real-client sequence).
func FuzzFrameStream(f *testing.F) {
	// Seed 1: client preface only.
	f.Add([]byte(clientPrefaceBytes))

	// Seed 2: preface + minimal SETTINGS frame (type=0x04, flags=0x00, stream=0, len=0).
	seed2 := make([]byte, 0, len(clientPrefaceBytes)+9)
	seed2 = append(seed2, clientPrefaceBytes...)
	seed2 = append(seed2,
		0x00, 0x00, 0x00, // length = 0
		0x04,                   // type = SETTINGS
		0x00,                   // flags = 0
		0x00, 0x00, 0x00, 0x00, // stream id = 0
	)
	f.Add(seed2)

	// Seed 3: preface + SETTINGS + SETTINGS ACK (flags=0x01).
	seed3 := make([]byte, 0, len(clientPrefaceBytes)+18)
	seed3 = append(seed3, clientPrefaceBytes...)
	// SETTINGS frame (empty payload)
	seed3 = append(seed3,
		0x00, 0x00, 0x00, // length = 0
		0x04,                   // type = SETTINGS
		0x00,                   // flags = 0
		0x00, 0x00, 0x00, 0x00, // stream id = 0
	)
	// SETTINGS ACK frame (flags=0x01)
	seed3 = append(seed3,
		0x00, 0x00, 0x00, // length = 0
		0x04,                   // type = SETTINGS
		0x01,                   // flags = ACK
		0x00, 0x00, 0x00, 0x00, // stream id = 0
	)
	f.Add(seed3)

	f.Fuzz(func(t *testing.T, input []byte) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		conn := newReplayConn(input)
		defer func() { _ = conn.Close() }()

		disp := stubDispatcher{}
		sc := NewServerConn(ctx, conn, disp, DefaultServerSettings)
		err := sc.Run()
		// No panic: assured by reaching here.
		if err == nil {
			return
		}
		// Context errors are acceptable.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return
		}
		// io.EOF and io.ErrUnexpectedEOF are acceptable: fuzz inputs are often
		// truncated, causing the underlying frame reader to return EOF before any
		// protocol-level validation can produce an h2:-prefixed error.
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return
		}
		// Every other error must begin with "h2:".
		if !strings.HasPrefix(err.Error(), "h2:") {
			t.Errorf("error %q does not begin with 'h2:' (and is not a context error)", err.Error())
		}
	})
}

// FuzzHPACKDecode wraps hpackState.decodeBlock with adversarial input.
// The underlying x/net/http2/hpack package has its own fuzzer; this is a
// wrapper-level integration test for our usage patterns and error translation.
//
// Per ADR-0018: 30-second short-budget in CI. Two seed entries:
//  1. Empty block (valid — decoder returns zero fields).
//  2. Well-formed single-pseudo-header block (:method = GET).
func FuzzHPACKDecode(f *testing.F) {
	// Seed 1: empty block.
	f.Add([]byte{})

	// Seed 2: well-formed encoded ":method: GET" using a fresh hpackState.
	seed2 := newHPACKState(4096).encodeHeaders([]hpack.HeaderField{
		{Name: ":method", Value: "GET"},
	})
	// encodeHeaders returns an alias of an internal buffer; copy before storing.
	seed2Copy := make([]byte, len(seed2))
	copy(seed2Copy, seed2)
	f.Add(seed2Copy)

	f.Fuzz(func(t *testing.T, block []byte) {
		st := newHPACKState(4096)
		_, err := st.decodeBlock(block, true)
		if err == nil {
			return
		}
		if !strings.HasPrefix(err.Error(), "h2:") {
			t.Errorf("error %q does not begin with 'h2:'", err.Error())
		}
		// No panic: assured by reaching here.
	})
}

// replayConn is an in-memory net.Conn whose Read side is backed by
// bytes.Reader (returns the seed/mutated bytes then EOF) and whose Write side
// discards all output. All deadline methods are no-ops and addr stubs return
// zero-value TCPAddr. This lets ServerConn drive a full frame-loop without any
// real network I/O.
type replayConn struct {
	r      *bytes.Reader
	w      *bytes.Buffer
	closed bool
}

func newReplayConn(b []byte) *replayConn {
	return &replayConn{r: bytes.NewReader(b), w: &bytes.Buffer{}}
}

func (c *replayConn) Read(p []byte) (int, error)  { return c.r.Read(p) }
func (c *replayConn) Write(p []byte) (int, error) { return c.w.Write(p) }
func (c *replayConn) Close() error                { c.closed = true; return nil }

func (c *replayConn) LocalAddr() net.Addr  { return &net.TCPAddr{} }
func (c *replayConn) RemoteAddr() net.Addr { return &net.TCPAddr{} }

func (c *replayConn) SetDeadline(t time.Time) error      { return nil }
func (c *replayConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *replayConn) SetWriteDeadline(t time.Time) error { return nil }

// stubDispatcher is a minimal Dispatcher implementation that satisfies the
// h2.Dispatcher interface. It returns ok=true with a stub 404 action so
// dispatch goroutines proceed without triggering INTERNAL_ERROR (which would
// happen on ok=false). The stub action writes a minimal 404 HEADERS frame.
type stubDispatcher struct{}

func (stubDispatcher) Match(_ *http.Request) (Action, bool) {
	return stubAction{}, true
}

// stubAction writes a minimal HTTP/2 404 response and ends the stream.
type stubAction struct{}

func (stubAction) WriteH2(_ context.Context, _ H2Request, sw StreamWriter) error {
	headers := []hpack.HeaderField{
		{Name: ":status", Value: "404"},
	}
	return sw.WriteHeaders(headers, true)
}

// fuzzRespHeaderStreamID is the non-zero stream id threaded through
// FuzzValidateResponseHeaderBlock. Zero would be connection-scoped.
const fuzzRespHeaderStreamID = uint32(11)

// oracleResponseHeaderBlockReject is an INDEPENDENTLY-WRITTEN re-statement of
// the leading-block rule set, used as the accept/reject oracle for
// FuzzValidateResponseHeaderBlock. It returns whether the block must be
// rejected plus the name and value of the field that causes it.
//
// ⚠️ IT DELIBERATELY CALLS NONE OF THE PREDICATES THE VALIDATOR CALLS. An
// oracle sharing isConnectionSpecificField, hasUppercaseHeaderChar or
// teTrailersValue would prove only that a function equals itself: dropping
// `upgrade` from the shared set would drop it from BOTH sides and the fuzzer
// would stay green. The closed name set, the uppercase test and the "trailers"
// literal are therefore all written out longhand here.
func oracleResponseHeaderBlockReject(fields []hpack.HeaderField) (reject bool, name, value string) {
	contentLengthCount := 0
	for _, hf := range fields {
		// RFC 9113 §8.2.1 — written out over BYTES rather than by calling
		// hasUppercaseHeaderChar. (Byte-wise and rune-wise agree: an ASCII
		// byte in 'A'..'Z' is never part of a multi-byte UTF-8 sequence.)
		for i := 0; i < len(hf.Name); i++ {
			if hf.Name[i] >= 'A' && hf.Name[i] <= 'Z' {
				return true, hf.Name, hf.Value
			}
		}
		// RFC 9113 §8.2.2 — the closed set, spelled out longhand.
		switch hf.Name {
		case "connection", "keep-alive", "proxy-connection", "transfer-encoding", "upgrade":
			return true, hf.Name, hf.Value
		case "te":
			// The ONLY legal value is "trailers" — written as a literal, not
			// as teTrailersValue. A present-but-EMPTY te is a rejection.
			if hf.Value != "trailers" {
				return true, hf.Name, hf.Value
			}
		case "content-length":
			contentLengthCount++
			if contentLengthCount > 1 {
				return true, hf.Name, hf.Value
			}
		}
	}
	return false, "", ""
}

// trailingQuotedToken returns the strconv.Quote-d token a rejection Msg ends
// with. Call it on *Error.Msg, NOT on Error(): Error() appends
// ": " + Underlying.Error() after the Msg, so the quoted token is trailing in
// the Msg only. The opening quote is found by FORWARD scan because no leg's
// fixed prefix contains a double quote, so the first one in the Msg opens the
// token; a backward scan would mis-anchor on an ESCAPED quote inside a field
// name or value the fuzzer produced.
func trailingQuotedToken(msg string) (string, bool) {
	if !strings.HasSuffix(msg, `"`) {
		return "", false
	}
	for i := 0; i < len(msg); i++ {
		if msg[i] != '"' {
			continue
		}
		if s, err := strconv.Unquote(msg[i:]); err == nil {
			return s, true
		}
		return "", false
	}
	return "", false
}

// FuzzValidateResponseHeaderBlock mutates HPACK-encoded LEADING response header
// blocks, decodes them, and drives validateResponseHeaders.
//
// ⚠️ REACHABILITY IS NOT COVERAGE, which is why this target is not redundant
// with the two above. FuzzFrameStream transitively reaches
// isConnectionSpecificField via buildRequest, but its only assertion is "no
// panic + every error begins with h2:" — it can NEVER observe a wrong
// classification. FuzzHPACKDecode reaches no predicate at all. Before this
// target the encode/response direction had no fuzz reach whatsoever.
//
// Per ADR-0018: 30-second short-budget in CI. Three assertions:
//  1. no panic (assured by reaching the end of the body);
//  2. every rejection message carries the "h2:" prefix AND names the offending
//     field QUOTED, in TRAILING position — the falsifiability discipline
//     client.go documents for the rule set;
//  3. the accept/reject verdict agrees with oracleResponseHeaderBlockReject, an
//     independently-written statement of the closed rule set that shares NO
//     predicate with the validator.
//
// Seeds: a bare legal 200, a legal 200 with a content-type, each of the eight
// single-field reject shapes, and a duplicate-content-length pair.
func FuzzValidateResponseHeaderBlock(f *testing.F) {
	addSeed := func(fields []hpack.HeaderField) {
		block := newHPACKState(4096).encodeHeaders(fields)
		// encodeHeaders returns an alias of an internal buffer; copy before storing.
		cp := make([]byte, len(block))
		copy(cp, block)
		f.Add(cp)
	}
	status := hpack.HeaderField{Name: ":status", Value: "200"}
	addSeed([]hpack.HeaderField{status})
	addSeed([]hpack.HeaderField{status, {Name: "content-type", Value: "text/plain"}})
	addSeed([]hpack.HeaderField{status, {Name: "connection", Value: "keep-alive"}})
	addSeed([]hpack.HeaderField{status, {Name: "transfer-encoding", Value: "chunked"}})
	addSeed([]hpack.HeaderField{status, {Name: "keep-alive", Value: "timeout=5"}})
	addSeed([]hpack.HeaderField{status, {Name: "upgrade", Value: "websocket"}})
	addSeed([]hpack.HeaderField{status, {Name: "proxy-connection", Value: "keep-alive"}})
	addSeed([]hpack.HeaderField{status, {Name: "X-Upper-Case", Value: "yes"}})
	addSeed([]hpack.HeaderField{status, {Name: "te", Value: "gzip"}})
	addSeed([]hpack.HeaderField{status, {Name: "te", Value: ""}})
	addSeed([]hpack.HeaderField{status, {Name: "content-length", Value: "5"}, {Name: "content-length", Value: "5"}})

	f.Fuzz(func(t *testing.T, block []byte) {
		fields, derr := newHPACKState(4096).decodeBlock(block, true)
		if derr != nil {
			// Adversarial input the HPACK layer rejects is out of scope here;
			// FuzzHPACKDecode owns that surface.
			return
		}
		got := validateResponseHeaders(fuzzRespHeaderStreamID, fields)
		wantReject, offName, offValue := oracleResponseHeaderBlockReject(fields)

		// (3) verdict parity with the independent oracle.
		if (got != nil) != wantReject {
			t.Errorf("validateResponseHeaders rejected=%v, oracle rejected=%v, fields=%+v (err=%v)",
				got != nil, wantReject, fields, got)
			return
		}
		if got == nil {
			// (1) no panic: assured by reaching here.
			return
		}
		// (2) the message shape. The "h2:" prefix is a property of the rendered
		// Error(); the QUOTED-and-TRAILING field name is a property of the
		// validator's own Msg — Error() appends ": " + the sentinel's text after
		// it, so the quoted token is trailing in Msg, not in Error().
		if !strings.HasPrefix(got.Error(), "h2:") {
			t.Errorf("error %q does not begin with 'h2:'", got.Error())
		}
		tok, ok := trailingQuotedToken(got.Msg)
		if !ok {
			t.Errorf("message %q does not end with a strconv.Quote-d token naming the offending field", got.Msg)
			return
		}
		if tok != offName && tok != offValue {
			t.Errorf("message %q names %q, want the offending field's name (%q) or value (%q)",
				got.Msg, tok, offName, offValue)
		}
	})
}
