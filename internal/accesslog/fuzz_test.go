package accesslog

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func FuzzAccessLogFormat(f *testing.F) {
	f.Add("GET", "/x", "HTTP/1.1", "host", "ua", "10.0.0.1:80")
	f.Add("\nGET", "/x\ny", "HTTP/1.1", "h\nx", "ua\n", "10.0.0.1:80")
	f.Add("GET", `/x"y`, "HTTP/1.1", `h"x`, `ua"y`, `host"port`)
	f.Add("\x00GET", "/\x00", "HTTP/1.1", "\x00", "\x00", "\x00")
	f.Add("GET", strings.Repeat("a", 2048), "HTTP/1.1", "h", "ua", "h:p")
	f.Add("\xff\x80\x81", "/\x90\x91", "HTTP/1.1", "h", "ua", "h:p")

	f.Fuzz(func(t *testing.T, method, path, proto, authority, ua, upstream string) {
		rec := &Record{
			StartTime:    time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
			Method:       method,
			Path:         path,
			Protocol:     proto,
			ResponseCode: 200,
			BytesSent:    42,
			Duration:     5 * time.Millisecond,
			Authority:    authority,
			UserAgent:    ua,
			UpstreamHost: upstream,
		}
		var got []byte
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Default panicked: %v", r)
				}
			}()
			got = Default(rec)
		}()
		body := bytes.TrimSuffix(got, []byte{'\n'})
		if bytes.IndexByte(body, '\n') >= 0 {
			t.Fatalf("embedded LF in record body: %q (input method=%q path=%q)", got, method, path)
		}
		quoteCount := 0
		for i := 0; i < len(got); i++ {
			if got[i] == '"' && (i == 0 || got[i-1] != '\\') {
				quoteCount++
			}
		}
		if quoteCount%2 != 0 {
			t.Fatalf("odd number of un-escaped quotes (%d): %q", quoteCount, got)
		}
	})
}
