package hcm

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
)

// emitAccessLog constructs an accesslog.Record from H1 primitives and submits
// to each sink in f.accessLog. Per SPEC §2.1, a zero statusCode is the H2
// ctx-cancel sentinel and skips emission; H1 path never produces a zero
// statusCode in normal flow, but the guard is uniform across H1+H2 callers.
func (f *Filter) emitAccessLog(r *http.Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time, respHeaders filter_http.OrderedHeaders) {
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
	f.captureRecordHeaders(rec, reqHeaderLookupH1(r), respHeaderLookup(respHeaders))
	for _, s := range f.accessLog {
		s.Submit(rec)
	}
}

// emitAccessLogH2 is the H2-flavored variant; reads pseudo-headers (:method,
// :path, :authority) and User-Agent from H2Request fields. Per SPEC §2.1
// last bullet, a zero statusCode is the H2 ctx-cancel sentinel and skips
// emission.
func (f *Filter) emitAccessLogH2(req h2.H2Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time, respHeaders filter_http.OrderedHeaders) {
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
	f.captureRecordHeaders(rec, reqHeaderLookupH2(req), respHeaderLookup(respHeaders))
	for _, s := range f.accessLog {
		s.Submit(rec)
	}
}

// captureRecordHeaders fills rec.RequestHeaders / rec.ResponseHeaders from the
// configured capture union via the supplied lookups. It allocates a map ONLY
// when the corresponding union is non-empty, so the no-capture path (empty
// unions) leaves both maps nil — byte-stable per D-HDR-RECORD-CAPTURE-SCOPE.
// A nil respLookup (or nil response carrier) yields no response capture.
func (f *Filter) captureRecordHeaders(rec *accesslog.Record, reqLookup, respLookup func(string) ([]string, bool)) {
	if len(f.alsReqHeaderNames) > 0 {
		rec.RequestHeaders = accesslog.CaptureHeaders(f.alsReqHeaderNames, reqLookup)
	}
	if len(f.alsRespHeaderNames) > 0 && respLookup != nil {
		rec.ResponseHeaders = accesslog.CaptureHeaders(f.alsRespHeaderNames, respLookup)
	}
}

// reqHeaderLookupH1 adapts http.Header for CaptureHeaders. http.Header.Values
// canonicalizes the name and returns all values in insertion order; an absent
// header yields (nil, false), a present-but-empty header yields ([""], true) —
// presence is the discriminator (AMEND-HDR-2).
func reqHeaderLookupH1(r *http.Request) func(string) ([]string, bool) {
	return func(name string) ([]string, bool) {
		v := r.Header.Values(name)
		return v, v != nil
	}
}

// reqHeaderLookupH2 scans the H2Request.Headers slice case-insensitively,
// COLLECTING all matching values in wire order (not first-match). Returns
// (nil, false) when no field matches (the absent-header omit case).
func reqHeaderLookupH2(req h2.H2Request) func(string) ([]string, bool) {
	return func(name string) ([]string, bool) {
		var vals []string
		for _, hf := range req.Headers {
			if strings.EqualFold(hf.Name, name) {
				vals = append(vals, hf.Value)
			}
		}
		return vals, vals != nil
	}
}

// respHeaderLookup scans an OrderedHeaders carrier case-insensitively,
// COLLECTING all matching values in wire order (OrderedHeaders.Get returns only
// the FIRST). A nil carrier (the error-site path where no response exists)
// returns a nil lookup, so captureRecordHeaders skips response capture entirely
// and leaves rec.ResponseHeaders nil. Used for BOTH H1 and H2 responses.
func respHeaderLookup(oh filter_http.OrderedHeaders) func(string) ([]string, bool) {
	if oh == nil {
		return nil
	}
	return func(name string) ([]string, bool) {
		var vals []string
		for _, hf := range oh {
			if strings.EqualFold(hf.Name, name) {
				vals = append(vals, hf.Value)
			}
		}
		return vals, vals != nil
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
