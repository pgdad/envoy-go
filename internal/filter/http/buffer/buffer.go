package buffer

import (
	"fmt"
	"net/http"

	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// TypeURL is the canonical envoy.filters.http.buffer typed_config type URL.
// Boot wiring in cmd/envoy-go/main.go registers New under this key in the
// HTTPRegistry per ADR-0072.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.buffer.v3.Buffer"

// filterName is the canonical http_filters[].name string for buffer
// (matches the listener config typed_per_filter_config map keys).
const filterName = "envoy.filters.http.buffer"

// cap1MiB is the envoy-go-side parse-time ceiling for max_request_bytes.
// Mirrors internal/filter/http/chain.go:19 filterBufferLimitBytes (1 << 20)
// per ADR-0126 cap-layering rationale.
const cap1MiB uint32 = 1 << 20 // 1048576

// compiledConfig captures the single consumed field from the Buffer proto
// per SPEC §6.1 + ADR-0126. No *filterStats field — phase 13 emits no
// filter-specific counters (SPEC §1.1 amendment 5).
type compiledConfig struct {
	maxRequestBytes uint32
}

// compiledPerRoute captures the parsed-and-validated per-route override per
// SPEC §6.3 + ADR-0125 5th canonical per-route discipline. Exactly one of
// disabled or maxOverride is meaningful at a time:
//   - disabled=true, maxOverride=nil: filter wholly inactive on this route.
//   - disabled=false, maxOverride=&v: max_request_bytes override for this route.
type compiledPerRoute struct {
	disabled    bool
	maxOverride *uint32
}

// filter is the per-instance per-stream filter state. Per ADR-0071's single-
// goroutine-per-stream invariant, the per-instance state is race-free without
// synchronization. The config pointer is closure-captured at New time and is
// immutable post-construction.
//
// Fields set in DecodeHeaders (Task 3):
//   - effectiveMax: resolved per-stream cap (listener or per-route override)
//   - passthrough: true when per-route config disables the filter on this route
//   - headersRef: reference to the held request headers for maybeAddContentLength
//
// Field accumulated in DecodeData (Task 4):
//   - accumulated: running byte count across all data chunks (uint32, matches maxRequestBytes)
type filter struct {
	config       *compiledConfig
	dcb          envoyhttp.DecoderFilterCallbacks
	effectiveMax uint32      // resolved cap; set in DecodeHeaders; read in DecodeData
	passthrough  bool        // true when per-route disabled; branched on in DecodeData
	headersRef   http.Header // held reference; set in DecodeHeaders; used by maybeAddContentLength
	accumulated  uint32      // running byte count across all data chunks; set in DecodeData
}

// Statically assert decoder-only conformance per planner-time decision 1.
var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)

// New is the HTTPFilterFactory exposed at boot. Per SPEC §6.1 + ADR-0126,
// New PGV-mirrors max_request_bytes (non-nil + > 0 + ≤ 1 MiB).
func New(tc *anypb.Any, _ envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, fmt.Errorf("buffer: invalid typed_config: nil")
	}
	cfg := &bufferv3.Buffer{}
	if err := tc.UnmarshalTo(cfg); err != nil {
		return nil, fmt.Errorf("buffer: invalid typed_config: %w", err)
	}
	if cfg.GetMaxRequestBytes() == nil {
		return nil, fmt.Errorf("buffer: max_request_bytes is required")
	}
	v := cfg.GetMaxRequestBytes().GetValue()
	if v == 0 {
		return nil, fmt.Errorf("buffer: max_request_bytes must be > 0")
	}
	if v > cap1MiB {
		return nil, fmt.Errorf("buffer: max_request_bytes (%d) exceeds envoy-go cap of %d bytes", v, cap1MiB)
	}
	cc := &compiledConfig{maxRequestBytes: v}
	return func() envoyhttp.HTTPFilter {
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: &filter{config: cc},
			Encoder: nil, // decoder-only per planner-time decision 5
		}
	}, nil
}

// parsePerRoute validates and compiles a BufferPerRoute proto.Message per
// SPEC §6.3 + ADR-0125 5th canonical per-route discipline. Called at request
// time from DecodeHeaders when RequestRouteConfig() returns a non-nil TPFC
// (body lands in Task 3).
func parsePerRoute(perRoute proto.Message) (*compiledPerRoute, error) {
	cfg, ok := perRoute.(*bufferv3.BufferPerRoute)
	if !ok {
		return nil, fmt.Errorf("buffer per-route: expected *BufferPerRoute, got %T", perRoute)
	}
	switch override := cfg.GetOverride().(type) {
	case *bufferv3.BufferPerRoute_Disabled:
		if !override.Disabled {
			return nil, fmt.Errorf("buffer per-route: disabled must be true (PGV bool.const violation)")
		}
		return &compiledPerRoute{disabled: true}, nil
	case *bufferv3.BufferPerRoute_Buffer:
		v := override.Buffer.GetMaxRequestBytes()
		if v == nil {
			return nil, fmt.Errorf("buffer per-route: max_request_bytes is required")
		}
		if v.GetValue() == 0 {
			return nil, fmt.Errorf("buffer per-route: max_request_bytes must be > 0")
		}
		if v.GetValue() > cap1MiB {
			return nil, fmt.Errorf("buffer per-route: max_request_bytes (%d) exceeds envoy-go cap of %d bytes", v.GetValue(), cap1MiB)
		}
		n := v.GetValue()
		return &compiledPerRoute{maxOverride: &n}, nil
	case nil:
		return nil, fmt.Errorf("buffer per-route: override oneof is required (neither disabled nor buffer set)")
	default:
		return nil, fmt.Errorf("buffer per-route: unknown override case %T", override)
	}
}

// --- Filter method skeletons (bodies land in Tasks 3-4) ---

func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) {
	f.dcb = cb
}

// DecodeHeaders implements the per-SPEC §6.4 entry point for the body-counting
// algorithm. Per ADR-0127 v2 + envoy-go HCM synchronous dispatch discipline:
//
//	(i) Header-only fast-path (endStream=true): mirrors buffer_filter.cc:54-56.
//	    No Content-Length inspection per §11.6 empirical pin.
//	(ii) Per-route disabled bypass: set passthrough + Continue (mirrors buffer_filter.cc:60-62).
//	(iii) Bodied + non-disabled: store effectiveMax + headersRef; return Continue.
//	     envoy-go's HCM dispatches RunDecodeHeaders + body loop sequentially in one
//	     goroutine (ADR-0076), so StopIteration here would deadlock (chain parks
//	     waiting for ContinueDecoding while the body loop can't proceed). Returning
//	     Continue passes headers through to the router filter; overflow detection
//	     fires in DecodeData, which accumulates bytes each chunk — same observable
//	     behavior as reference Envoy's buffer_filter.cc.
func (f *filter) DecodeHeaders(headers http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
	// Step 1: Header-only fast-path (mirrors buffer_filter.cc:54-56).
	if endStream {
		return envoyhttp.Continue
	}
	// Step 2: Resolve effectiveMax + disabled flag.
	effectiveMax, disabled := f.resolveEffective()
	// Step 3: Per-route disabled bypass (mirrors buffer_filter.cc:60-62).
	if disabled {
		f.passthrough = true
		return envoyhttp.Continue
	}
	// Step 4: Bodied + non-disabled — hold headers; return Continue (see doc above).
	f.effectiveMax = effectiveMax
	f.headersRef = headers
	return envoyhttp.Continue
}

// resolveEffective resolves the effective max_request_bytes cap for this stream.
// Returns (cap, disabled). Listener fallback applies when no per-route config is present.
// Per ADR-0127 v2 §Decision (i): called once per stream from DecodeHeaders; result is
// cached in f.effectiveMax + f.passthrough for use by DecodeData (Task 4).
func (f *filter) resolveEffective() (effectiveMax uint32, disabled bool) {
	if f.dcb == nil {
		return f.config.maxRequestBytes, false
	}
	resolved := f.dcb.RequestRouteConfig() // returns proto.Message or nil
	if resolved == nil {
		return f.config.maxRequestBytes, false
	}
	cpr, err := parsePerRoute(resolved)
	if err != nil {
		// Unparseable per-route — fall back to listener config.
		return f.config.maxRequestBytes, false
	}
	if cpr.disabled {
		return 0, true
	}
	if cpr.maxOverride != nil {
		return *cpr.maxOverride, false
	}
	return f.config.maxRequestBytes, false
}

// DecodeData implements the body-counting algorithm per ADR-0127 v2 + envoy-go
// HCM synchronous dispatch discipline:
//
//	(i)   Per-route disabled bypass: passthrough returns DataContinue without accumulating.
//	(ii)  Accumulate: f.accumulated += len(data).
//	(iii) Overflow check (strict >): emit 413 + DataStopIterationNoBuffer.
//	(iv)  Terminal chunk fits: invoke maybeAddContentLength; return DataContinue.
//	(v)   In-flight chunk: return DataContinue. envoy-go's HCM accumulates all body
//	     bytes in connection.go's bodyBuf before RunAction dials the upstream (ADR-0076),
//	     so DataStopIterationAndBuffer is not needed — the framework already holds
//	     the bytes. DataContinue just passes the chunk through without parking.
func (f *filter) DecodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
	// Step 1: Per-route disabled bypass.
	if f.passthrough {
		return envoyhttp.DataContinue
	}
	// Step 2: Accumulate.
	f.accumulated += uint32(len(data))
	// Step 3: Mid-stream overflow — emit 413 + discard partial buffer.
	if f.accumulated > f.effectiveMax {
		f.dcb.SendLocalReply(413, "Payload Too Large", envoyhttp.OrderedHeaders{
			{Name: "Connection", Value: "close"},
		})
		return envoyhttp.DataStopIterationNoBuffer
	}
	// Step 4: Terminal chunk fits — invoke maybeAddContentLength; release held headers + body.
	if endStream {
		f.maybeAddContentLength()
		return envoyhttp.DataContinue
	}
	// Step 5: In-flight chunk — return DataContinue (see doc above).
	return envoyhttp.DataContinue
}

// maybeAddContentLength mirrors buffer_filter.cc:91-97. When the request had no
// Content-Length (chunked transfer encoding), injects Content-Length: <accumulated>
// and drops Transfer-Encoding: chunked. No-op when headersRef is nil or when
// Content-Length is already present (idempotent per §11.8).
func (f *filter) maybeAddContentLength() {
	if f.headersRef != nil && f.headersRef.Get("Content-Length") == "" {
		f.headersRef.Set("Content-Length", fmt.Sprintf("%d", f.accumulated))
		// Per §11.8 empirical evidence: Envoy ALSO drops Transfer-Encoding: chunked.
		f.headersRef.Del("Transfer-Encoding")
	}
}

// DecodeTrailers defensively invokes maybeAddContentLength in case end-stream
// arrived via trailers rather than via terminal endStream=true on data.
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	f.maybeAddContentLength()
	return envoyhttp.TrailersContinue
}

func (f *filter) OnDestroy() {}
