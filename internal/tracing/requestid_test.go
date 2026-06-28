package tracing

import "testing"

func TestRequestIDReasonNibble(t *testing.T) {
	cases := []struct {
		reason TraceReason
		want   byte
	}{
		{NoTrace, '4'},
		{Sampled, '9'},
		{Client, 'b'},
	}
	for _, c := range cases {
		if got := reasonNibble(c.reason); got != c.want {
			t.Fatalf("reasonNibble(%v) = %q, want %q", c.reason, got, c.want)
		}
	}
}

func TestRequestIDReasonNibbleUnknownDefaultsToNoTrace(t *testing.T) {
	if got := reasonNibble(TraceReason(99)); got != '4' {
		t.Fatalf("reasonNibble(unknown) = %q, want '4'", got)
	}
}

// sixteenBytes is a fixed 16-byte fill for the Generate tests.
func sixteenBytes() []byte {
	return []byte{
		0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
		0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10,
	}
}

func assertUUIDShape(t *testing.T, out string) {
	t.Helper()
	if len(out) != 36 {
		t.Fatalf("len = %d, want 36 (%q)", len(out), out)
	}
	for _, i := range []int{8, 13, 18, 23} {
		if out[i] != '-' {
			t.Fatalf("index %d = %q, want '-' (%q)", i, out[i], out)
		}
	}
	// Canonical variant nibble at string index 19 must be 8/9/a/b.
	switch out[19] {
	case '8', '9', 'a', 'b':
	default:
		t.Fatalf("variant nibble at index 19 = %q, want one of 8/9/a/b (%q)", out[19], out)
	}
}

func TestRequestIDGenerateSampled(t *testing.T) {
	fake := &fakeRand{bytes: sixteenBytes()}
	out := GenerateRequestID(Sampled, fake)
	assertUUIDShape(t, out)
	if out[14] != '9' {
		t.Fatalf("index 14 = %q, want '9' (%q)", out[14], out)
	}
}

func TestRequestIDGenerateNoTrace(t *testing.T) {
	fake := &fakeRand{bytes: sixteenBytes()}
	out := GenerateRequestID(NoTrace, fake)
	assertUUIDShape(t, out)
	if out[14] != '4' {
		t.Fatalf("index 14 = %q, want '4' (%q)", out[14], out)
	}
}

func TestRequestIDGenerateClient(t *testing.T) {
	fake := &fakeRand{bytes: sixteenBytes()}
	out := GenerateRequestID(Client, fake)
	assertUUIDShape(t, out)
	if out[14] != 'b' {
		t.Fatalf("index 14 = %q, want 'b' (%q)", out[14], out)
	}
}

func TestRequestIDStampOverwritesOnlyIndex14(t *testing.T) {
	in := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	got := StampRequestID(in, Sampled)
	want := "aaaaaaaa-bbbb-9ccc-dddd-eeeeeeeeeeee"
	if got != want {
		t.Fatalf("StampRequestID = %q, want %q", got, want)
	}
}

func TestRequestIDStampMalformedShortUnchanged(t *testing.T) {
	for _, in := range []string{"", "short", "aaaaaaaa-bbbb"} {
		if got := StampRequestID(in, Sampled); got != in {
			t.Fatalf("StampRequestID(%q) = %q, want unchanged", in, got)
		}
	}
}

func FuzzStampRequestID(f *testing.F) {
	f.Add("")
	f.Add("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	f.Add("short")
	f.Fuzz(func(t *testing.T, s string) {
		out := StampRequestID(s, Sampled) // must not panic
		if len(s) >= 15 {
			if out[14] != '9' {
				t.Fatalf("index 14 = %q, want '9'", out[14])
			}
			if out[:14] != s[:14] || out[15:] != s[15:] {
				t.Fatalf("stamp mutated bytes outside index 14")
			}
		} else if out != s {
			t.Fatalf("short input mutated: %q -> %q", s, out)
		}
	})
}
