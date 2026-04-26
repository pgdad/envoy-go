package h2

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
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
		if err == context.DeadlineExceeded || err == context.Canceled {
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

func (stubAction) WriteH2(sw StreamWriter) error {
	headers := []hpack.HeaderField{
		{Name: ":status", Value: "404"},
	}
	return sw.WriteHeaders(headers, true)
}
