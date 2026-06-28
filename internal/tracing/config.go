// Package tracing is the HCM-native request-tracing engine (the FIRST genuinely-new
// request-path package per the phase-46.1a plan). At 46.1a it establishes the parsed
// TracingConfig + the RandSource randomness seam consumed by the sampling/request-id
// decision engine; the OTLP span exporter + Resource land at 46.1b (ADR-0260).
package tracing

import (
	"fmt"

	tracev3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
)

// TracingConfig is the parsed HCM tracing config (D-TRACE-CONFIG-HOME — lives here,
// built from the HCM proto by NewConfig in Task 5, consumed by Decide). The three
// Percent knobs default to 100.0 when ABSENT (the reference default); an EXPLICIT 0
// is a valid 0% and is preserved by the parse. ServiceName/ClusterName are stored
// for the 46.1b exporter/Resource and are UNUSED at 46.1a.
//
//nolint:revive // D-TRACE-CONFIG-HOME (ADR-0260) reserves the tracing.TracingConfig name for the parsed-HCM-tracing-config surface.
type TracingConfig struct {
	ClientSampling  float64
	RandomSampling  float64
	OverallSampling float64
	ServiceName     string
	ClusterName     string
}

// otelTypeName is the proto full-name of the only supported tracing provider's
// typed_config (envoy.config.trace.v3.OpenTelemetryConfig). Every other provider
// type is rejected loudly (envoy-go-strict, ADR-0080) — no silent-ignore of a
// sibling provider Any (reference_strict_reject_sibling_typeurl_gap).
var otelTypeName = (&tracev3.OpenTelemetryConfig{}).ProtoReflect().Descriptor().FullName()

// NewConfig parses the typed HCM `tracing` message into a TracingConfig, applying
// the envoy-go-strict posture (ADR-0080): the HCM tracing field is lifted from the
// ADR silent-ignore set, but every unsupported sub-feature is rejected loudly. A nil
// message means tracing is not configured and returns (nil, nil) — the byte-stable
// no-tracing path. Only the OpenTelemetry provider over an `envoy_grpc` transport is
// supported; the three sampling knobs default to 100% when absent (an explicit 0% is
// preserved).
func NewConfig(t *hcmv3.HttpConnectionManager_Tracing) (*TracingConfig, error) {
	if t == nil {
		return nil, nil
	}

	if t.GetVerbose() {
		return nil, fmt.Errorf("tracing: verbose is unsupported")
	}
	if t.GetMaxPathTagLength() != nil {
		return nil, fmt.Errorf("tracing: max_path_tag_length is unsupported")
	}
	if len(t.GetCustomTags()) > 0 {
		return nil, fmt.Errorf("tracing: custom_tags unsupported")
	}
	if t.GetSpawnUpstreamSpan() != nil {
		return nil, fmt.Errorf("tracing: spawn_upstream_span unsupported")
	}

	p := t.GetProvider()
	if p == nil {
		return nil, fmt.Errorf("tracing: provider required")
	}
	tc := p.GetTypedConfig()
	if tc == nil {
		return nil, fmt.Errorf("tracing: provider typed_config required")
	}
	if tc.MessageName() != otelTypeName {
		return nil, fmt.Errorf("tracing: provider %s unsupported (only OpenTelemetry)", tc.GetTypeUrl())
	}

	var otel tracev3.OpenTelemetryConfig
	if err := proto.Unmarshal(tc.GetValue(), &otel); err != nil {
		return nil, fmt.Errorf("tracing: unmarshal OpenTelemetryConfig: %w", err)
	}

	if otel.GetHttpService() != nil {
		return nil, fmt.Errorf("tracing: http_service unsupported")
	}
	if len(otel.GetResourceDetectors()) > 0 {
		return nil, fmt.Errorf("tracing: resource_detectors unsupported")
	}
	if otel.GetSampler() != nil {
		return nil, fmt.Errorf("tracing: sampler unsupported")
	}

	gs := otel.GetGrpcService()
	if gs.GetGoogleGrpc() != nil {
		return nil, fmt.Errorf("tracing: google_grpc transport unsupported")
	}
	cluster := gs.GetEnvoyGrpc().GetClusterName()
	if cluster == "" {
		return nil, fmt.Errorf("tracing: empty grpc_service cluster_name")
	}

	pct := func(p *typev3.Percent, def float64) float64 {
		if p == nil {
			return def
		}
		return p.GetValue()
	}

	return &TracingConfig{
		ClientSampling:  pct(t.GetClientSampling(), 100),
		RandomSampling:  pct(t.GetRandomSampling(), 100),
		OverallSampling: pct(t.GetOverallSampling(), 100),
		ServiceName:     otel.GetServiceName(),
		ClusterName:     cluster,
	}, nil
}
