package hcm

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

// emitAccessLog constructs an accesslog.Record from H1 primitives and submits
// to each sink in f.accessLog. Per SPEC §2.1, a zero statusCode is the H2
// ctx-cancel sentinel and skips emission; H1 path never produces a zero
// statusCode in normal flow, but the guard is uniform across H1+H2 callers.
func (f *Filter) emitAccessLog(r *http.Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time) {
	if statusCode == 0 || len(f.accessLog) == 0 {
		return
	}
	rec := &accesslog.Record{
		StartTime:    start,
		Method:       r.Method,
		Path:         r.URL.Path,
		Protocol:     r.Proto,
		ResponseCode: statusCode,
		BytesSent:    bytesSent,
		Duration:     time.Since(start),
		Authority:    r.Host,
		UserAgent:    r.Header.Get("User-Agent"),
		UpstreamHost: upstreamHostString(picked),
	}
	for _, s := range f.accessLog {
		s.Submit(rec)
	}
}

// emitAccessLogH2 is the H2-flavored variant; reads pseudo-headers (:method,
// :path, :authority) and User-Agent from H2Request fields. Per SPEC §2.1
// last bullet, a zero statusCode is the H2 ctx-cancel sentinel and skips
// emission.
func (f *Filter) emitAccessLogH2(req h2.H2Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time) {
	if statusCode == 0 || len(f.accessLog) == 0 {
		return
	}
	rec := &accesslog.Record{
		StartTime:    start,
		Method:       req.Method,
		Path:         req.Path,
		Protocol:     "HTTP/2.0",
		ResponseCode: statusCode,
		BytesSent:    bytesSent,
		Duration:     time.Since(start),
		Authority:    req.Authority,
		UserAgent:    h2UserAgent(req),
		UpstreamHost: upstreamHostString(picked),
	}
	for _, s := range f.accessLog {
		s.Submit(rec)
	}
}

// h2UserAgent extracts the User-Agent header value from an H2Request's
// Headers slice (case-insensitive match per RFC 7540 §8.1.2 — header names
// are lowercase in HTTP/2). Returns empty string if absent.
func h2UserAgent(req h2.H2Request) string {
	for _, hf := range req.Headers {
		if strings.EqualFold(hf.Name, "user-agent") {
			return hf.Value
		}
	}
	return ""
}

// upstreamHostString renders cluster.Endpoint as `host:port` for the access-log
// UPSTREAM_HOST operator. Zero-valued Endpoint (host == "") yields empty
// string; the formatter then emits the literal `-` per Decision A.
func upstreamHostString(ep cluster.Endpoint) string {
	if ep.Host == "" {
		return ""
	}
	return ep.Host + ":" + strconv.Itoa(int(ep.Port))
}
