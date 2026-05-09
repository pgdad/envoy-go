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
// immutable post-construction. Fields effectiveMax, accumulated, passthrough,
// headersRef are added in Tasks 3-4 when the body-counting body lands.
type filter struct {
	config *compiledConfig
	dcb    envoyhttp.DecoderFilterCallbacks
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
		if v := override.Buffer.GetMaxRequestBytes(); v == nil {
			return nil, fmt.Errorf("buffer per-route: max_request_bytes is required")
		} else if v.GetValue() == 0 {
			return nil, fmt.Errorf("buffer per-route: max_request_bytes must be > 0")
		} else if v.GetValue() > cap1MiB {
			return nil, fmt.Errorf("buffer per-route: max_request_bytes (%d) exceeds envoy-go cap of %d bytes", v.GetValue(), cap1MiB)
		} else {
			n := v.GetValue()
			return &compiledPerRoute{maxOverride: &n}, nil
		}
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

// DecodeHeaders skeleton — body lands in Task 3.
func (f *filter) DecodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	return envoyhttp.Continue
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
