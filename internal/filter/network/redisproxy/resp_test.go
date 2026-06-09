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
	cmd, raw, err := decodeRequest(r)
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
	cmd, raw, err := decodeRequest(reqReader("PING\n"))
	if err != nil || cmd != "PING" || string(raw) != "PING\n" {
		t.Fatalf("cmd=%q raw=%q err=%v", cmd, raw, err)
	}
}

func TestDecodeRequest_ArrayOfBulk(t *testing.T) {
	wire := "*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"
	cmd, raw, err := decodeRequest(reqReader(wire))
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
	cmd, _, err := decodeRequest(reqReader("*1\r\n$4\r\nping\r\n"))
	if err != nil || cmd != "PING" {
		t.Fatalf("cmd = %q err=%v, want PING (uppercased)", cmd, err)
	}
}

func TestDecodeRequest_EOFAtFrameBoundary(t *testing.T) {
	// An empty reader → clean io.EOF (the connection ended between frames).
	_, _, err := decodeRequest(reqReader(""))
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
	cmd, _, err := decodeRequest(bufio.NewReader(pr))
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
			if _, _, err := decodeRequest(reqReader(s)); err == nil {
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
