package accesslog

import (
	"bytes"
	"strconv"
	"strings"
)

// Default formats a Record per the Envoy v1.37.2 default access-log format
// (per ADR-0066 + SPEC §6 + §11 empirical pin). The 15 operators in identical
// positions on every record:
//
//	[%START_TIME%] "%REQ(:METHOD)% %REQ(X-ENVOY-ORIGINAL-PATH?:PATH)% %PROTOCOL%" %RESPONSE_CODE% %RESPONSE_FLAGS% %BYTES_RECEIVED% %BYTES_SENT% %DURATION% %RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)% "%REQ(X-FORWARDED-FOR)%" "%REQ(USER-AGENT)%" "%REQ(X-REQUEST-ID)%" "%REQ(:AUTHORITY)%" "%UPSTREAM_HOST%"
//
// Per Decision A's option-B partial coverage, the 5 unplumbed operators
// (RESPONSE_FLAGS, BYTES_RECEIVED, RESP(X-ENVOY-UPSTREAM-SERVICE-TIME),
// X-FORWARDED-FOR, X-REQUEST-ID) emit the literal `-` (Envoy missing-value
// convention). Quoted operators escape literal `"` to `\"` per Envoy convention.
// The line is terminated with a single `\n`; embedded LFs in any field are
// stripped (replaced with `\n` literal escape) so the line-stream invariant
// load-bearing for the fixture-0006 parser holds (per SPEC §1 #10 + Decision J).
func Default(r *Record) []byte {
	var b bytes.Buffer
	b.Grow(256)
	b.WriteByte('[')
	b.WriteString(r.StartTime.UTC().Format("2006-01-02T15:04:05.000Z"))
	b.WriteByte(']')
	b.WriteString(` "`)
	b.WriteString(escape(r.Method))
	b.WriteByte(' ')
	b.WriteString(escape(orDash(r.Path)))
	b.WriteByte(' ')
	b.WriteString(escape(r.Protocol))
	b.WriteByte('"')
	b.WriteByte(' ')
	b.WriteString(strconv.Itoa(r.ResponseCode))
	b.WriteString(" - -")
	b.WriteByte(' ')
	b.WriteString(strconv.FormatInt(r.BytesSent, 10))
	b.WriteByte(' ')
	b.WriteString(strconv.FormatInt(int64(r.Duration/1e6), 10))
	b.WriteString(` -`)
	b.WriteString(` "-"`)
	b.WriteString(` "`)
	b.WriteString(escape(orEmptyDash(r.UserAgent)))
	b.WriteByte('"')
	b.WriteString(` "-"`)
	b.WriteString(` "`)
	b.WriteString(escape(orEmptyDash(r.Authority)))
	b.WriteByte('"')
	b.WriteString(` "`)
	b.WriteString(escape(orEmptyDash(r.UpstreamHost)))
	b.WriteByte('"')
	b.WriteByte('\n')
	return b.Bytes()
}

func escape(s string) string {
	if !strings.ContainsAny(s, "\"\n\r") {
		return s
	}
	r := strings.NewReplacer(`"`, `\"`, "\n", `\n`, "\r", `\r`)
	return r.Replace(s)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func orEmptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
