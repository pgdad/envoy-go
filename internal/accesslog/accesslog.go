// Package accesslog provides envoy-go's access-log subsystem: an in-tree file-sink
// and Envoy-default-format formatter, plus an async-writer with bounded-channel
// drop-newest backpressure. Per ADR-0066, the package is a thin in-tree primitive
// with no third-party access-log dependency.
//
// Lifecycle: Sinks are opened in cmd/envoy-go/main.go between bootstrap.Load and
// listener.Run; threaded into the HCM filter chain via internal/filter/hcm/config.go;
// closed via defer sink.Close() after listener.Shutdown returns. SIGTERM-while-pending
// drain semantics are Phase 08's deliverable.
package accesslog

import "time"

// Sink is an access-log destination. Implementations include AsyncFileSink (the
// only sink type in 06.2; future phases may add ALS / OTLP). Submit is non-blocking
// (drop-newest backpressure on full channel; see writer.go); Close is idempotent
// and threadsafe (sync.Once-guarded; see writer.go).
type Sink interface {
	Submit(r any)
	Close() error
}

// Record is the per-request primitives populated by HCM at finalization-time and
// consumed by the Default formatter. Per Decision A (option B partial-with-`-`)
// the 10 fields below cover the 10 plumbed operators; the 5 unplumbed operators
// (RESPONSE_FLAGS, BYTES_RECEIVED, RESP(X-ENVOY-UPSTREAM-SERVICE-TIME),
// X-FORWARDED-FOR, X-REQUEST-ID) are emitted as the literal `-` by the formatter
// without needing Record fields.
type Record struct {
	StartTime    time.Time
	Method       string
	Path         string
	Protocol     string
	ResponseCode int
	BytesSent    int64
	Duration     time.Duration
	Authority    string
	UserAgent    string
	UpstreamHost string

	RequestHeaders  map[string]string // captured additional_request_headers_to_log (lowercase key, comma-joined value); nil when no capture configured
	ResponseHeaders map[string]string // captured additional_response_headers_to_log (lowercase key, comma-joined value); nil when no capture configured
}
