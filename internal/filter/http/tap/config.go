package tap

import (
	"errors"
	"fmt"

	taptapv3 "github.com/envoyproxy/go-control-plane/envoy/config/tap/v3"
	commontapv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/tap/v3"
	httptapv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/tap/v3"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/matchpredicate"
	"github.com/pgdad/envoy-go/internal/stats"
)

// TypeURL is the tap filter's typed_config @type.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.tap.v3.Tap"

const filterName = "envoy.filters.http.tap"

// config is the immutable, per-listener compiled tap configuration. It is
// shared by every *tapFilter minted for a stream.
type config struct {
	prog       *matchpredicate.Program
	sink       *filePerTapSink
	rqTapped   *stats.Counter
	recordConn bool
}

// New is the ADR-0071 two-step factory: it parses + compiles once, then returns
// a closure that mints one *tapFilter per stream.
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	cfg, err := parseConfig(tc, ctx)
	if err != nil {
		return nil, err
	}
	return func() envoyhttp.HTTPFilter {
		f := &tapFilter{cfg: cfg}
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: f,
			Encoder: f, // SAME *tapFilter: chain.Destroy()'s `else if` makes an
			// encoder-only OnDestroy unreachable (the compressor precedent).
		}
	}, nil
}

// parseConfig parses and compiles tc into a config. See SPEC §6 for the
// PARITY/DEPARTURE reject roster it enforces.
func parseConfig(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (*config, error) {
	if tc == nil {
		return nil, errors.New("tap: typed_config required")
	}
	var t httptapv3.Tap
	if err := tc.UnmarshalTo(&t); err != nil {
		return nil, fmt.Errorf("tap: unmarshal Tap: %w", err)
	}

	// f2: no landed accessor exposes the per-direction header-arrival instant.
	if t.GetRecordHeadersReceivedTime() {
		return nil, errors.New("tap: record_headers_received_time is not supported " +
			"(no per-direction header-arrival instant is exposed to HTTP filters)")
	}

	cc := t.GetCommonConfig()
	if cc == nil {
		return nil, errors.New("tap: common_config required")
	}
	switch a := cc.GetConfigType().(type) {
	case *commontapv3.CommonExtensionConfig_StaticConfig:
		// accepted below
	case *commontapv3.CommonExtensionConfig_AdminConfig:
		return nil, errors.New("tap: common_config.admin_config is not supported")
	case nil:
		return nil, errors.New("tap: common_config has no config_type set")
	default:
		return nil, fmt.Errorf("tap: unsupported common_config arm %T", a)
	}
	sc := cc.GetStaticConfig()
	if sc == nil {
		return nil, errors.New("tap: static_config required")
	}

	if sc.GetMatchConfig() != nil { //nolint:staticcheck // SA1019: arm EXISTS to PARSE-REJECT this deprecated proto field; intentional access.
		return nil, errors.New("tap: match_config (deprecated) is not supported; use match")
	}
	if sc.GetTapEnabled() != nil {
		return nil, errors.New("tap: tap_enabled is not supported")
	}
	if sc.GetMatch() == nil {
		// PARITY with the reference: "Neither match nor match_config is set in TapConfig".
		return nil, errors.New("tap: neither match nor match_config is set in TapConfig")
	}
	prog, err := matchpredicate.Compile(sc.GetMatch())
	if err != nil {
		return nil, fmt.Errorf("tap: match: %w", err)
	}

	oc := sc.GetOutputConfig()
	if oc == nil {
		return nil, errors.New("tap: output_config required")
	}
	if oc.GetStreaming() {
		return nil, errors.New("tap: output_config.streaming=true is not supported")
	}
	// PARITY with PGV: OutputConfig.sinks must contain exactly 1 item.
	if n := len(oc.GetSinks()); n != 1 {
		return nil, fmt.Errorf("tap: output_config.sinks must contain exactly 1 item(s), got %d", n)
	}
	s := oc.GetSinks()[0]

	switch f := s.GetFormat(); f {
	case taptapv3.OutputSink_JSON_BODY_AS_STRING, taptapv3.OutputSink_JSON_BODY_AS_BYTES:
		// Indistinguishable at 56.1: `body` is never populated. 56.2 diverges.
	case taptapv3.OutputSink_PROTO_BINARY,
		taptapv3.OutputSink_PROTO_BINARY_LENGTH_DELIMITED,
		taptapv3.OutputSink_PROTO_TEXT:
		return nil, fmt.Errorf("tap: output_config.sinks[0].format %v is not supported", f)
	default:
		return nil, fmt.Errorf("tap: unknown output_config.sinks[0].format %v", f)
	}

	switch a := s.GetOutputSinkType().(type) {
	case *taptapv3.OutputSink_FilePerTap:
		// accepted below
	case *taptapv3.OutputSink_StreamingAdmin:
		return nil, errors.New("tap: streaming_admin sink is not supported")
	case *taptapv3.OutputSink_BufferedAdmin:
		return nil, errors.New("tap: buffered_admin sink is not supported")
	case *taptapv3.OutputSink_StreamingGrpc:
		return nil, errors.New("tap: streaming_grpc sink is not supported")
	case *taptapv3.OutputSink_CustomSink:
		return nil, errors.New("tap: custom_sink is not supported")
	case nil:
		return nil, errors.New("tap: output_config.sinks[0] has no output_sink_type set")
	default:
		return nil, fmt.Errorf("tap: unsupported output_sink_type %T", a)
	}
	prefix := s.GetFilePerTap().GetPathPrefix()
	if prefix == "" {
		return nil, errors.New("tap: file_per_tap.path_prefix required")
	}
	sink, err := newFilePerTapSink(prefix)
	if err != nil {
		return nil, fmt.Errorf("tap: %w", err)
	}

	cfg := &config{prog: prog, sink: sink, recordConn: t.GetRecordDownstreamConnection()}
	// Registered at filter-parse (not per-stream) so it reads 0 with no taps.
	// The `.tap.` segment is HARDCODED, not the http_filters[] entry name.
	if ctx.Stats != nil {
		cfg.rqTapped = ctx.Stats.NewCounter("http." + ctx.StatPrefix + ".tap.rq_tapped")
	}
	return cfg, nil
}
