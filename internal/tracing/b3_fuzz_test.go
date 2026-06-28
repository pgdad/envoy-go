package tracing

import (
	"net/http"
	"testing"
)

// FuzzExtractB3 drives ExtractB3 over the UNTRUSTED b3 / X-B3-* boundary. The
// corpus fuzzes the single "b3" header string plus the individual X-B3-* fields;
// the body asserts ExtractB3 never panics and that a successful parse never yields
// a zero TraceID or zero ParentID (the all-zero guards).
func FuzzExtractB3(f *testing.F) {
	// b3, x-b3-traceid, x-b3-spanid, x-b3-parentspanid, x-b3-sampled, x-b3-flags
	f.Add("0102030405060708-1112131415161718-1", "", "", "", "", "")                                    // valid 64-bit single
	f.Add("0102030405060708090a0b0c0d0e0f10-1112131415161718-1-2122232425262728", "", "", "", "", "")   // valid 128-bit 4-field
	f.Add("0102030405060708-1112131415161718", "", "", "", "", "")                                      // 2-field deferred
	f.Add("", "0102030405060708090a0b0c0d0e0f10", "1112131415161718", "2122232425262728", "1", "")      // valid multi
	f.Add("", "0102030405060708", "1112131415161718", "", "", "1")                                      // multi debug flags
	f.Add("", "", "", "", "", "")                                                                       // empty
	f.Add("garbage", "", "", "", "", "")                                                                // junk single
	f.Add("0000000000000000-1112131415161718-1", "", "", "", "", "")                                    // all-zero trace-id
	f.Add("0102030405060708-1112131415161718-1-2122232425262728-extra", "zzz", "qqq", "----", "x", "9") // malformed mix

	f.Fuzz(func(t *testing.T, b3, tid, sid, pid, sampled, flags string) {
		h := http.Header{}
		if b3 != "" {
			h.Set("b3", b3)
		}
		if tid != "" {
			h.Set("X-B3-TraceId", tid)
		}
		if sid != "" {
			h.Set("X-B3-SpanId", sid)
		}
		if pid != "" {
			h.Set("X-B3-ParentSpanId", pid)
		}
		if sampled != "" {
			h.Set("X-B3-Sampled", sampled)
		}
		if flags != "" {
			h.Set("X-B3-Flags", flags)
		}

		ctx, ok := ExtractB3(h) // must not panic
		if ok {
			if ctx.TraceID == ([16]byte{}) {
				t.Fatalf("ok parse with zero TraceID: b3=%q tid=%q sid=%q", b3, tid, sid)
			}
			if ctx.ParentID == ([8]byte{}) {
				t.Fatalf("ok parse with zero ParentID: b3=%q tid=%q sid=%q pid=%q", b3, tid, sid, pid)
			}
		}
	})
}
