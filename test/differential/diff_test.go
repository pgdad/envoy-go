package differential

import "testing"

func TestCompareBytes_Equal(t *testing.T) {
	v, err := CompareBytes([]byte("hello"), []byte("hello"))
	if err != nil {
		t.Fatalf("CompareBytes: %v", err)
	}
	if !v.Equal {
		t.Errorf("verdict: %+v; want Equal=true", v)
	}
}

func TestCompareBytes_DivergesAtFirstByte(t *testing.T) {
	v, err := CompareBytes([]byte("hello"), []byte("Hello"))
	if err != nil {
		t.Fatalf("CompareBytes: %v", err)
	}
	if v.Equal {
		t.Errorf("verdict: %+v; want Equal=false", v)
	}
	if v.FirstDiffOffset != 0 {
		t.Errorf("FirstDiffOffset: got %d, want 0", v.FirstDiffOffset)
	}
	if v.HexDump == "" {
		t.Errorf("HexDump empty")
	}
}

func TestCompareBytes_DifferentLengths(t *testing.T) {
	v, _ := CompareBytes([]byte("hello"), []byte("hello!"))
	if v.Equal {
		t.Errorf("verdict: %+v; want Equal=false", v)
	}
	if v.FirstDiffOffset != 5 {
		t.Errorf("FirstDiffOffset: got %d, want 5", v.FirstDiffOffset)
	}
}
