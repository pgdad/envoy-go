package redisproxy

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
)

func reqReader(s string) *bufio.Reader { return bufio.NewReader(strings.NewReader(s)) }

func TestDecodeRequest_InlinePing(t *testing.T) {
	r := reqReader("PING\r\n")
	cmd, _, raw, err := decodeRequest(r)
	if err != nil {
		t.Fatalf("decodeRequest: %v", err)
	}
	if cmd != "PING" {
		t.Errorf("cmd = %q, want PING", cmd)
	}
	if string(raw) != "PING\r\n" {
		t.Errorf("raw = %q, want %q", raw, "PING\r\n")
	}
}

func TestDecodeRequest_InlineBareLF(t *testing.T) {
	// inline accepts a bare \n terminator (no \r).
	cmd, _, raw, err := decodeRequest(reqReader("PING\n"))
	if err != nil || cmd != "PING" || string(raw) != "PING\n" {
		t.Fatalf("cmd=%q raw=%q err=%v", cmd, raw, err)
	}
}

func TestDecodeRequest_ArrayOfBulk(t *testing.T) {
	wire := "*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"
	cmd, _, raw, err := decodeRequest(reqReader(wire))
	if err != nil {
		t.Fatalf("decodeRequest: %v", err)
	}
	if cmd != "SET" {
		t.Errorf("cmd = %q, want SET", cmd)
	}
	if string(raw) != wire {
		t.Errorf("raw = %q, want verbatim %q", raw, wire)
	}
}

func TestDecodeRequest_CaseInsensitiveCommand(t *testing.T) {
	cmd, _, _, err := decodeRequest(reqReader("*1\r\n$4\r\nping\r\n"))
	if err != nil || cmd != "PING" {
		t.Fatalf("cmd = %q err=%v, want PING (uppercased)", cmd, err)
	}
}

func TestDecodeRequest_EOFAtFrameBoundary(t *testing.T) {
	// An empty reader → clean io.EOF (the connection ended between frames).
	_, _, _, err := decodeRequest(reqReader(""))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("err = %v, want io.EOF at the frame boundary", err)
	}
}

func TestDecodeRequest_PartialFrameBlocksThenResumes(t *testing.T) {
	// A pipe that delivers a request in two writes proves block-and-resume: the
	// decoder blocks mid-frame on the first short read and completes on the second.
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("*1\r\n$4\r\nPI"))
		_, _ = pw.Write([]byte("NG\r\n"))
		_ = pw.Close()
	}()
	cmd, _, _, err := decodeRequest(bufio.NewReader(pr))
	if err != nil || cmd != "PING" {
		t.Fatalf("cmd=%q err=%v, want PING across two reads", cmd, err)
	}
}

func TestDecodeRequest_MalformedNeverPanics(t *testing.T) {
	bad := []string{
		"*abc\r\n",               // non-numeric array count
		"*2\r\n$3\r\nSET\r\n",    // truncated mid-array (declares 2, supplies 1) → unexpected EOF
		"*1\r\n$-5\r\n",          // negative non(-1) bulk length
		"*1\r\n$99999999999\r\n", // overflow bulk length (> maxBulkLen)
		"$3\r\nfoo\r\n",          // a reply type byte where a request is expected
		"*1\r\n#bad\r\n",         // bad bulk type marker
	}
	for _, s := range bad {
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("decodeRequest(%q) PANICKED: %v", s, p)
				}
			}()
			if _, _, _, err := decodeRequest(reqReader(s)); err == nil {
				t.Errorf("decodeRequest(%q) = nil error, want a decode error", s)
			}
		}()
	}
}

func replyRaw(t *testing.T, s string) string {
	t.Helper()
	raw, err := decodeReply(reqReader(s))
	if err != nil {
		t.Fatalf("decodeReply(%q): %v", s, err)
	}
	return string(raw)
}

func TestDecodeReply_ValueTypes(t *testing.T) {
	for _, wire := range []string{
		"+OK\r\n",                       // simple string
		"-ERR bad\r\n",                  // error
		":42\r\n",                       // integer
		"$3\r\nbar\r\n",                 // bulk string
		"$-1\r\n",                       // null bulk
		"*-1\r\n",                       // null array
		"*2\r\n$3\r\nfoo\r\n:7\r\n",     // array with mixed elements
		"*2\r\n*1\r\n+a\r\n$1\r\nb\r\n", // NESTED array
	} {
		if got := replyRaw(t, wire); got != wire {
			t.Errorf("decodeReply(%q) raw = %q, want verbatim", wire, got)
		}
	}
}

func TestDecodeReply_MalformedNeverPanics(t *testing.T) {
	bad := []string{"%bad\r\n", "$\r\n", ":x\r\n", "*1\r\n", "$5\r\nab\r\n"}
	for _, s := range bad {
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("decodeReply(%q) PANICKED: %v", s, p)
				}
			}()
			if _, err := decodeReply(reqReader(s)); err == nil {
				t.Errorf("decodeReply(%q) = nil error, want a decode error", s)
			}
		}()
	}
}

func TestLocalReplyConstants_ByteStable(t *testing.T) {
	if string(respPong) != "+PONG\r\n" {
		t.Errorf("respPong = %q, want +PONG\\r\\n", respPong)
	}
	if string(respAuthNoPassword) != "-ERR Client sent AUTH, but no password is set\r\n" {
		t.Errorf("respAuthNoPassword = %q", respAuthNoPassword)
	}
}

func TestDecodeRequest_ExposesArgs(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantCmd string
		wantArg []string // args[0]=command token (original case), args[1:]=arguments
	}{
		{"array SET", "*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n", "SET", []string{"SET", "foo", "bar"}},
		{"array ECHO", "*2\r\n$4\r\nECHO\r\n$2\r\nhi\r\n", "ECHO", []string{"ECHO", "hi"}},
		{"inline PING arg", "PING hello\r\n", "PING", []string{"PING", "hello"}},
		{"array HELLO 3", "*2\r\n$5\r\nHELLO\r\n$1\r\n3\r\n", "HELLO", []string{"HELLO", "3"}},
		{"lowercase echo preserved in args0", "*1\r\n$3\r\nget\r\n", "GET", []string{"get"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, args, _, err := decodeRequest(bufio.NewReader(strings.NewReader(tc.in)))
			if err != nil {
				t.Fatalf("decodeRequest(%q) err = %v", tc.in, err)
			}
			if cmd != tc.wantCmd {
				t.Errorf("cmd = %q, want %q", cmd, tc.wantCmd)
			}
			if len(args) != len(tc.wantArg) {
				t.Fatalf("len(args) = %d, want %d (%q)", len(args), len(tc.wantArg), args)
			}
			for i := range tc.wantArg {
				if string(args[i]) != tc.wantArg[i] {
					t.Errorf("args[%d] = %q, want %q", i, args[i], tc.wantArg[i])
				}
			}
		})
	}
}

// The reply-array depth guard: nesting at the limit parses; one level beyond
// returns errProtocol (stack-exhaustion guard — a chained "*1\r\n" reply at 4
// bytes/level would otherwise recurse until the goroutine stack dies, an
// unrecoverable runtime panic).
func TestDecodeReply_NestingDepthLimit(t *testing.T) {
	deep := strings.Repeat("*1\r\n", maxReplyDepth-1) + ":1\r\n"
	raw, err := decodeReply(reqReader(deep))
	if err != nil {
		t.Fatalf("nesting at the limit (depth %d) must parse: %v", maxReplyDepth-1, err)
	}
	if string(raw) != deep {
		t.Errorf("raw = %q, want the verbatim frame", raw)
	}
	tooDeep := strings.Repeat("*1\r\n", maxReplyDepth) + ":1\r\n"
	if _, err := decodeReply(reqReader(tooDeep)); !errors.Is(err, errProtocol) {
		t.Fatalf("err = %v, want errProtocol beyond maxReplyDepth", err)
	}
}
