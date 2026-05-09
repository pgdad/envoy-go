package buffer

import (
	"fmt"
	"net/http"

	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
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
//   - headersRef: reference to the held request headers for maybeAddContentLength (Task 4)
//
// Field added in DecodeData (Task 4):
//   - accumulated: running byte count across all data chunks
type filter struct {
	config       *compiledConfig
	dcb          envoyhttp.DecoderFilterCallbacks
	effectiveMax uint32      // resolved cap; set in DecodeHeaders; read in DecodeData
	passthrough  bool        // true when per-route disabled; branched on in DecodeData
	headersRef   http.Header // held reference; set in DecodeHeaders; used by maybeAddContentLength (Task 4)
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
// algorithm. Per ADR-0127 v2:
//
//	(i) Header-only fast-path (endStream=true): mirrors buffer_filter.cc:54-56.
//	    No Content-Length inspection per §11.6 empirical pin.
//	(ii) Per-route disabled bypass: set passthrough + Continue (mirrors buffer_filter.cc:60-62).
//	(iii) Bodied + non-disabled: store effectiveMax + headersRef; return StopIteration
//	     (mirrors buffer_filter.cc:67). Task 4 completes DecodeData + maybeAddContentLength.
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
	// Step 4: Bodied + non-disabled — hold headers; signal StopIteration (mirrors buffer_filter.cc:67).
	f.effectiveMax = effectiveMax
	f.headersRef = headers
	return envoyhttp.StopIteration
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

// DecodeData skeleton — body lands in Task 4.
func (f *filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// DecodeTrailers skeleton — body lands in Task 4.
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

func (f *filter) OnDestroy() {}
