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
	"google.golang.org/protobuf/types/known/anypb"
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
	Provider        ProviderKind
	Zipkin          *ZipkinSettings // non-nil iff Provider == ProviderZipkin
}

// ProviderKind names the parsed tracing provider (D-TRACE-ZIPKIN-CONFIG-SHAPE);
// NewConfig dispatches the typed_config Any onto one of these arms.
type ProviderKind int

// The supported tracing provider kinds (one per NewConfig dispatch arm).
const (
	ProviderOTel ProviderKind = iota
	ProviderZipkin
)

// ZipkinSettings is the parsed envoy.config.trace.v3.ZipkinConfig surface
// consumed by the Zipkin exporter. The collector cluster lives on the parent
// TracingConfig.ClusterName (shared with the OTel arm).
type ZipkinSettings struct {
	CollectorEndpoint string // the POST path (collector_endpoint)
	CollectorHostname string // the POST Host; empty => the cluster name (collector_hostname)
	TraceID128Bit     bool   // 32-hex traceId when true; 16-hex (low 64) otherwise
	SharedSpanContext bool   // absent => true (the Envoy default)
}

// otelTypeName / zipkinTypeName are the proto full-names of the two supported
// tracing providers' typed_config (envoy.config.trace.v3.OpenTelemetryConfig and
// .ZipkinConfig). Every other provider type is rejected loudly (envoy-go-strict,
// ADR-0080) — no silent-ignore of a sibling provider Any
// (reference_strict_reject_sibling_typeurl_gap).
var (
	otelTypeName   = (&tracev3.OpenTelemetryConfig{}).ProtoReflect().Descriptor().FullName()
	zipkinTypeName = (&tracev3.ZipkinConfig{}).ProtoReflect().Descriptor().FullName()
)

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

	pct := func(p *typev3.Percent, def float64) float64 {
		if p == nil {
			return def
		}
		return p.GetValue()
	}
	clientSampling := pct(t.GetClientSampling(), 100)
	randomSampling := pct(t.GetRandomSampling(), 100)
	overallSampling := pct(t.GetOverallSampling(), 100)

	switch tc.MessageName() {
	case otelTypeName:
		return parseOTel(tc, clientSampling, randomSampling, overallSampling)
	case zipkinTypeName:
		return parseZipkin(tc, clientSampling, randomSampling, overallSampling)
	default:
		return nil, fmt.Errorf("tracing: provider %s unsupported (only OpenTelemetry or Zipkin)", tc.GetTypeUrl())
	}
}

// parseOTel parses an OpenTelemetryConfig typed_config Any into a TracingConfig
// (Provider == ProviderOTel). The strict-reject arms are unchanged from 46.1a.
func parseOTel(tc *anypb.Any, clientSampling, randomSampling, overallSampling float64) (*TracingConfig, error) {
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

	return &TracingConfig{
		ClientSampling:  clientSampling,
		RandomSampling:  randomSampling,
		OverallSampling: overallSampling,
		ServiceName:     otel.GetServiceName(),
		ClusterName:     cluster,
		Provider:        ProviderOTel,
	}, nil
}

// parseZipkin parses a ZipkinConfig typed_config Any into a TracingConfig
// (Provider == ProviderZipkin). Only the HTTP_JSON collector endpoint version is
// supported; split_spans_for_request and an empty collector_cluster are rejected
// loudly (envoy-go-strict, ADR-0080).
func parseZipkin(tc *anypb.Any, clientSampling, randomSampling, overallSampling float64) (*TracingConfig, error) {
	var z tracev3.ZipkinConfig
	if err := proto.Unmarshal(tc.GetValue(), &z); err != nil {
		return nil, fmt.Errorf("tracing: unmarshal ZipkinConfig: %w", err)
	}

	if v := z.GetCollectorEndpointVersion(); v != tracev3.ZipkinConfig_HTTP_JSON {
		return nil, fmt.Errorf("tracing: zipkin collector_endpoint_version %v unsupported (only HTTP_JSON)", v)
	}
	if z.GetSplitSpansForRequest() { //nolint:staticcheck // SA1019: this arm EXISTS to PARSE-REJECT the deprecated split_spans_for_request; intentional access (ADR-0080, envoy-go-strict).
		return nil, fmt.Errorf("tracing: zipkin split_spans_for_request unsupported")
	}
	cluster := z.GetCollectorCluster()
	if cluster == "" {
		return nil, fmt.Errorf("tracing: empty zipkin collector_cluster")
	}

	shared := true
	if sv := z.GetSharedSpanContext(); sv != nil {
		shared = sv.GetValue()
	}

	return &TracingConfig{
		ClientSampling:  clientSampling,
		RandomSampling:  randomSampling,
		OverallSampling: overallSampling,
		ServiceName:     "",
		ClusterName:     cluster,
		Provider:        ProviderZipkin,
		Zipkin: &ZipkinSettings{
			CollectorEndpoint: z.GetCollectorEndpoint(),
			CollectorHostname: z.GetCollectorHostname(),
			TraceID128Bit:     z.GetTraceId_128Bit(),
			SharedSpanContext: shared,
		},
	}, nil
}
