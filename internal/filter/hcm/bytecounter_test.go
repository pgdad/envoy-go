package hcm

import (
	"bytes"
	"errors"
	"testing"
)

func TestByteCounterWriter_AccumulatesBytesWritten(t *testing.T) {
	var buf bytes.Buffer
	bcw := &byteCounterWriter{w: &buf}
	for _, p := range [][]byte{[]byte("hello "), []byte("world"), []byte("!")} {
		n, err := bcw.Write(p)
		if err != nil || n != len(p) {
			t.Errorf("Write(%q) = (%d, %v), want (%d, nil)", p, n, err, len(p))
		}
	}
	if bcw.n != 12 {
		t.Errorf("bcw.n = %d, want 12", bcw.n)
	}
}

type shortWriter struct{ limit int }

func (sw *shortWriter) Write(p []byte) (int, error) {
	if len(p) > sw.limit {
		return sw.limit, errors.New("short")
	}
	return len(p), nil
}

func TestByteCounterWriter_ShortWriteAccountsActualBytes(t *testing.T) {
	bcw := &byteCounterWriter{w: &shortWriter{limit: 3}}
	n, err := bcw.Write([]byte("hello"))
	if err == nil {
		t.Fatal("expected error")
	}
	if n != 3 {
		t.Errorf("n = %d, want 3", n)
	}
	if bcw.n != 3 {
		t.Errorf("bcw.n = %d, want 3", bcw.n)
	}
}
