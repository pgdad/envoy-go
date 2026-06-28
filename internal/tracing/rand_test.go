package tracing

import (
	"bytes"
	"testing"
)

// fakeRand is the deterministic RandSource test seam (reused by later tasks'
// tests). Float64 pops successive programmed values; Read copies from the
// programmed bytes.
type fakeRand struct {
	floats []float64
	bytes  []byte
}

func (f *fakeRand) Float64() float64 {
	if len(f.floats) == 0 {
		return 0
	}
	v := f.floats[0]
	f.floats = f.floats[1:]
	return v
}

func (f *fakeRand) Read(p []byte) (int, error) {
	n := copy(p, f.bytes)
	f.bytes = f.bytes[n:]
	return n, nil
}

// fakeRand must satisfy RandSource.
var _ RandSource = (*fakeRand)(nil)

func TestFakeRandFloat64PopsSuccessiveValues(t *testing.T) {
	f := &fakeRand{floats: []float64{0.1, 0.9}}
	if got := f.Float64(); got != 0.1 {
		t.Fatalf("first Float64 = %v, want 0.1", got)
	}
	if got := f.Float64(); got != 0.9 {
		t.Fatalf("second Float64 = %v, want 0.9", got)
	}
}

func TestFakeRandReadCopiesProgrammedBytes(t *testing.T) {
	want := []byte{1, 2, 3, 4}
	f := &fakeRand{bytes: append([]byte(nil), want...)}
	p := make([]byte, 4)
	n, err := f.Read(p)
	if err != nil {
		t.Fatalf("Read err = %v", err)
	}
	if n != 4 {
		t.Fatalf("Read n = %d, want 4", n)
	}
	if !bytes.Equal(p, want) {
		t.Fatalf("Read p = %v, want %v", p, want)
	}
}

func TestCryptoRandReadFillsAndIsNonDeterministic(t *testing.T) {
	var r CryptoRand

	a := make([]byte, 16)
	n, err := r.Read(a)
	if err != nil {
		t.Fatalf("Read err = %v", err)
	}
	if n != 16 {
		t.Fatalf("Read n = %d, want 16", n)
	}

	b := make([]byte, 16)
	if _, err := r.Read(b); err != nil {
		t.Fatalf("second Read err = %v", err)
	}
	if bytes.Equal(a, b) {
		t.Fatalf("two successive CryptoRand reads were identical: %v", a)
	}
}

func TestCryptoRandFloat64InUnitInterval(t *testing.T) {
	var r CryptoRand
	for i := 0; i < 1000; i++ {
		v := r.Float64()
		if v < 0 || v >= 1 {
			t.Fatalf("Float64 = %v, want in [0,1)", v)
		}
	}
}

// CryptoRand must satisfy RandSource.
var _ RandSource = CryptoRand{}
