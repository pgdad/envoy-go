package hcm

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pgdad/envoy-go/internal/accesslog"
	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/tracing"
)

// emitAccessLog constructs an accesslog.Record from H1 primitives and submits
// to each sink in f.accessLog. Per SPEC §2.1, a zero statusCode is the H2
// ctx-cancel sentinel and skips emission; H1 path never produces a zero
// statusCode in normal flow, but the guard is uniform across H1+H2 callers.
//
// Phase 46.1b Task 8: the span block PRECEDES the len(f.accessLog)==0
// early-return so that a tracing HCM with no access_log block still exports
// spans (AMEND-TRACE-SPANEND-SEAM). A non-zero statusCode + non-nil exporter +
// non-nil traceDecision with Sample==true triggers span build and export.
func (f *Filter) emitAccessLog(r *http.Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time, respHeaders filter_http.OrderedHeaders, traceDecision *tracing.Decision) {
	// SPAN BLOCK — must be BEFORE the access-log early-return (AMEND-TRACE-SPANEND-SEAM):
	// a tracing HCM without any access_log block still exports spans.
	if statusCode != 0 && f.exporter != nil && traceDecision != nil && traceDecision.Sample {
		reqSize := r.ContentLength
		if reqSize < 0 {
			reqSize = 0
		}
		// URL: scheme://authority/path-and-query.  H1 server-side requests do not
		// carry r.URL.Scheme (set by stdlib only for client-side requests); default
		// to "http" to match Envoy's behavior on plain listeners.
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		url := tracing.BuildHTTPURL(scheme, r.Host, r.URL.RequestURI(), f.tracingConfig.MaxPathTagLength)
		in := tracing.SpanInputs{
			Method:            r.Method,
			URL:               url,
			Protocol:          r.Proto,
			StatusCode:        statusCode,
			UserAgent:         r.Header.Get("User-Agent"),
			RequestSize:       reqSize,
			ResponseSize:      bytesSent,
			UpstreamCluster:   "", // not available at this seam; Task 11 differential is UNasserted on this field
			DownstreamCluster: "-",
			ResponseFlags:     "-",
			ClientTraceID:     r.Header.Get("X-Client-Trace-Id"),
			Authority:         r.Host,
		}
		f.exporter.Export(tracing.BuildServerSpan(*traceDecision, in, tracing.ResolveCustomTags(f.tracingConfig.CustomTags, reqHeaderLookupH1(r)), start, time.Now()))
	}
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
//
// Phase 46.1b Task 8: symmetric span block at FUNCTION HEAD (same
// AMEND-TRACE-SPANEND-SEAM ordering as emitAccessLog above).
func (f *Filter) emitAccessLogH2(req h2.H2Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time, respHeaders filter_http.OrderedHeaders, traceDecision *tracing.Decision) {
	// SPAN BLOCK — must be BEFORE the access-log early-return (AMEND-TRACE-SPANEND-SEAM).
	if statusCode != 0 && f.exporter != nil && traceDecision != nil && traceDecision.Sample {
		// URL: scheme://authority/path.  H2 carries :scheme so use it directly.
		scheme := req.Scheme
		if scheme == "" {
			scheme = "https" // H2 defaults to https per RFC 9113 §8.3.1
		}
		url := tracing.BuildHTTPURL(scheme, req.Authority, req.Path, f.tracingConfig.MaxPathTagLength)
		// x-client-trace-id from the H2 regular headers (case-insensitive scan).
		var clientTraceID string
		for _, hf := range req.Headers {
			if strings.EqualFold(hf.Name, "x-client-trace-id") {
				clientTraceID = hf.Value
				break
			}
		}
		in := tracing.SpanInputs{
			Method:            req.Method,
			URL:               url,
			Protocol:          "HTTP/2.0",
			StatusCode:        statusCode,
			UserAgent:         h2UserAgent(req),
			RequestSize:       int64(len(req.Body)),
			ResponseSize:      bytesSent,
			UpstreamCluster:   "", // not available at this seam; UNasserted in differential
			DownstreamCluster: "-",
			ResponseFlags:     "-",
			ClientTraceID:     clientTraceID,
			Authority:         req.Authority,
		}
		f.exporter.Export(tracing.BuildServerSpan(*traceDecision, in, tracing.ResolveCustomTags(f.tracingConfig.CustomTags, reqHeaderLookupH2(req)), start, time.Now()))
	}
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

// emitAccessLogH3 is the HTTP/3 access-log + span arm (sibling of emitAccessLog
// (H1) / emitAccessLogH2). It reads from the native *http.Request (like the H1
// emitAccessLog) but forces Protocol="HTTP/3" in BOTH the span
// SpanInputs.Protocol and the Record.Protocol: the reference's %PROTOCOL% /
// :protocol() emit "HTTP/3", not r.Proto's "HTTP/3.0" (the 61.3 differential
// verifies the exact string cross-side). The span block precedes the
// access-log early-return (AMEND-TRACE-SPANEND-SEAM), identical ordering to
// emitAccessLog / emitAccessLogH2. Phase 61.2, ADR-0281.
func (f *Filter) emitAccessLogH3(r *http.Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time, respHeaders filter_http.OrderedHeaders, traceDecision *tracing.Decision) {
	// SPAN BLOCK — must be BEFORE the access-log early-return (AMEND-TRACE-SPANEND-SEAM).
	if statusCode != 0 && f.exporter != nil && traceDecision != nil && traceDecision.Sample {
		reqSize := r.ContentLength
		if reqSize < 0 {
			reqSize = 0
		}
		// URL: scheme://authority/path-and-query. HTTP/3 is TLS-only, so the
		// scheme is https whenever r.TLS is populated (quic-go always populates
		// it in production); mirror the H1 nil-tolerant default for the
		// unit-test path that leaves r.TLS unset.
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		url := tracing.BuildHTTPURL(scheme, r.Host, r.URL.RequestURI(), f.tracingConfig.MaxPathTagLength)
		in := tracing.SpanInputs{
			Method:            r.Method,
			URL:               url,
			Protocol:          "HTTP/3",
			StatusCode:        statusCode,
			UserAgent:         r.Header.Get("User-Agent"),
			RequestSize:       reqSize,
			ResponseSize:      bytesSent,
			UpstreamCluster:   "", // not available at this seam; UNasserted in differential
			DownstreamCluster: "-",
			ResponseFlags:     "-",
			ClientTraceID:     r.Header.Get("X-Client-Trace-Id"),
			Authority:         r.Host,
		}
		f.exporter.Export(tracing.BuildServerSpan(*traceDecision, in, tracing.ResolveCustomTags(f.tracingConfig.CustomTags, reqHeaderLookupH1(r)), start, time.Now()))
	}
	if statusCode == 0 || len(f.accessLog) == 0 {
		return
	}
	rec := &accesslog.Record{
		StartTime:    start,
		Method:       r.Method,
		Path:         r.URL.Path,
		Protocol:     "HTTP/3",
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
