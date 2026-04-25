package h2

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

const goodPreface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

func TestReadClientPreface_Good(t *testing.T) {
	r := bytes.NewReader([]byte(goodPreface))
	if err := readClientPreface(r); err != nil {
		t.Fatalf("readClientPreface(good) = %v, want nil", err)
	}
}

func TestReadClientPreface_BadByteAtEachPosition(t *testing.T) {
	for i := 0; i < len(goodPreface); i++ {
		buf := []byte(goodPreface)
		buf[i] ^= 0xff
		r := bytes.NewReader(buf)
		err := readClientPreface(r)
		if err == nil {
			t.Errorf("position %d: tampered preface accepted; want error", i)
			continue
		}
		if !strings.HasPrefix(err.Error(), "h2: PROTOCOL_ERROR") {
			t.Errorf("position %d: got %q, want h2:-PROTOCOL_ERROR-prefixed", i, err.Error())
		}
	}
}

func TestReadClientPreface_Truncated(t *testing.T) {
	r := bytes.NewReader([]byte(goodPreface[:10]))
	err := readClientPreface(r)
	if err == nil {
		t.Fatal("truncated preface accepted; want error")
	}
	if !strings.HasPrefix(err.Error(), "h2: PROTOCOL_ERROR") {
		t.Errorf("got %q, want h2:-PROTOCOL_ERROR-prefixed", err.Error())
	}
}

func TestReadClientPreface_EmptyEOF(t *testing.T) {
	r := bytes.NewReader(nil)
	err := readClientPreface(r)
	if err == nil {
		t.Fatal("empty preface accepted; want error")
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) && !strings.HasPrefix(err.Error(), "h2: PROTOCOL_ERROR") {
		t.Errorf("got %q, want EOF-wrapped or h2: PROTOCOL_ERROR", err.Error())
	}
}
