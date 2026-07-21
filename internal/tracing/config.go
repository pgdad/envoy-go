// Package tracing is the HCM-native request-tracing engine (the FIRST genuinely-new
// request-path package per the phase-46.1a plan). At 46.1a it establishes the parsed
// TracingConfig + the RandSource randomness seam consumed by the sampling/request-id
// decision engine; the OTLP span exporter + Resource land at 46.1b (ADR-0260).
package tracing

import (
	"fmt"

	tracev3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	tracingv3 "github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3"
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
	// MaxPathTagLength is the resolved byte-cap on the http.url span attribute's
	// :path (path+query) portion: the reference default 256 when ABSENT, the explicit
	// value otherwise (an explicit 0 = empty path is PRESERVED). ALWAYS set by
	// NewConfig, so a configured-tracing Filter never sees the zero value as a
	// spurious 0-cap (D-MPTL-DEFAULT / D-MPTL-ZERO, SPEC-64 §11).
	MaxPathTagLength uint32
	ServiceName      string
	ClusterName      string
	Provider         ProviderKind
	Zipkin           *ZipkinSettings // non-nil iff Provider == ProviderZipkin
	// CustomTags are the parsed custom tags (provider-neutral), ORDERED and
	// first-wins-deduplicated by tag key at parse (matching the reference's
	// config-time map). ResolveCustomTags resolves them per-request into span
	// attributes appended by BuildServerSpan (each overriding a colliding built-in).
	// Empty/nil when the HCM tracing block configures no custom_tags — the
	// byte-stable no-tags path.
	CustomTags []CustomTagSpec
}

// customTagKind selects a CustomTagSpec's value source: a static literal, a
// per-request lookup of a named downstream request header, a process env var, or
// a per-request lookup of a REQUEST-kind dynamic-metadata value.
type customTagKind uint8

const (
	kindLiteral customTagKind = iota
	kindRequestHeader
	kindEnvironment // reads a named process env var (os.LookupEnv), omit-on-empty-resolved
	kindMetadata    // reads a REQUEST-kind dynamic-metadata value (metadata_key.key + path), request_header default rule
)

// CustomTagSpec is one parsed HCM tracing custom_tag, resolved per-request by
// ResolveCustomTags. Kind selects the source: a static literal value, or a
// request-header lookup (with an optional default / omit-on-missing). The spec
// list on TracingConfig.CustomTags is ordered and first-wins-deduplicated by Key
// at parse (matching the reference's config-time map, SPEC-62 §11 arms C/D).
type CustomTagSpec struct {
	Key           string        // the span-attribute key (CustomTag.tag)
	Kind          customTagKind // kindLiteral | kindRequestHeader | kindEnvironment | kindMetadata
	LiteralValue  string        // Kind==kindLiteral: the static value
	HeaderName    string        // Kind==kindRequestHeader: the header to read
	EnvName       string        // Kind==kindEnvironment: the process env var to read (os.LookupEnv)
	MetaNamespace string        // Kind==kindMetadata: the dynamic-metadata namespace (metadata_key.key)
	MetaPath      []string      // Kind==kindMetadata: the pre-extracted path segment keys (min 1, validated at parse)
	DefaultValue  string        // Kind==kindRequestHeader | kindEnvironment | kindMetadata: value when the source is absent
	HasDefault    bool          // Kind==kindRequestHeader | kindMetadata: DefaultValue != "" (kindEnvironment uses the omit-iff-resolved-empty rule, not HasDefault)
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
	maxPathTagLen := uint32(256) // the reference default when ABSENT (D-MPTL-DEFAULT, SPEC-64 §11 arm 1)
	if m := t.GetMaxPathTagLength(); m != nil {
		maxPathTagLen = m.GetValue() // explicit value; an explicit 0 is PRESERVED (D-MPTL-ZERO, arm 2)
	}
	customTags, err := parseCustomTags(t.GetCustomTags())
	if err != nil {
		return nil, err
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

	var cfg *TracingConfig
	switch tc.MessageName() {
	case otelTypeName:
		cfg, err = parseOTel(tc, clientSampling, randomSampling, overallSampling)
	case zipkinTypeName:
		cfg, err = parseZipkin(tc, clientSampling, randomSampling, overallSampling)
	default:
		return nil, fmt.Errorf("tracing: provider %s unsupported (only OpenTelemetry or Zipkin)", tc.GetTypeUrl())
	}
	if err != nil {
		return nil, err
	}
	cfg.CustomTags = customTags
	cfg.MaxPathTagLength = maxPathTagLen
	return cfg, nil
}

// parseCustomTags converts the HCM tracing custom_tags into an ORDERED, first-wins-
// deduplicated []CustomTagSpec. Three source types are supported: `literal` (a static
// {tag, value} STRING attribute), `request_header` (the FIRST value of a named
// downstream request header, resolved per-request by ResolveCustomTags, with an
// optional default / omit-on-missing), and `environment` (the value of a named PROCESS
// ENVIRONMENT VARIABLE, resolved per-span by ResolveCustomTags via os.LookupEnv, with
// an optional default / omit-on-missing / omit-on-empty), and `metadata` (a REQUEST-
// kind dynamic-metadata value: namespace metadata_key.key + a path walk, resolved
// per-span by ResolveCustomTags, request_header default rule). The metadata ROUTE/
// CLUSTER/HOST/unset kinds reject LOUDLY with distinct substrings (envoy-go-strict
// DEPARTURE, ADR-0080 — the reference accepts those kinds). PGV-
// parity structural rejects (empty tag / empty literal.value / empty
// request_header.name / empty environment.name / empty metadata namespace / empty
// metadata path / empty metadata path segment / typeless) mirror the reference
// boot-reject (both reject — NOT a departure). All substrings are ADR-0080-distinct.
// Dedup runs AFTER per-tag
// structural validation, so a later duplicate-key tag with an invalid name still
// boot-rejects (parity with the reference PGV, which validates every entry before
// building the map). First-wins: the FIRST tag with a given key survives; a later
// same-key tag of ANY source type is dropped (SPEC-62 §11 arms C/D).
func parseCustomTags(tags []*tracingv3.CustomTag) ([]CustomTagSpec, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	out := make([]CustomTagSpec, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, ct := range tags {
		tag := ct.GetTag()
		if tag == "" {
			return nil, fmt.Errorf("tracing: custom_tags empty tag")
		}
		var spec CustomTagSpec
		switch {
		case ct.GetLiteral() != nil:
			v := ct.GetLiteral().GetValue()
			if v == "" {
				return nil, fmt.Errorf("tracing: custom_tags literal tag %q empty value", tag)
			}
			spec = CustomTagSpec{Key: tag, Kind: kindLiteral, LiteralValue: v}
		case ct.GetRequestHeader() != nil:
			h := ct.GetRequestHeader()
			if h.GetName() == "" {
				return nil, fmt.Errorf("tracing: custom_tags request_header tag %q empty name", tag)
			}
			dv := h.GetDefaultValue()
			spec = CustomTagSpec{Key: tag, Kind: kindRequestHeader, HeaderName: h.GetName(), DefaultValue: dv, HasDefault: dv != ""}
		case ct.GetEnvironment() != nil:
			e := ct.GetEnvironment()
			if e.GetName() == "" {
				return nil, fmt.Errorf("tracing: custom_tags environment tag %q empty name", tag)
			}
			spec = CustomTagSpec{Key: tag, Kind: kindEnvironment, EnvName: e.GetName(), DefaultValue: e.GetDefaultValue()}
		case ct.GetMetadata() != nil:
			md := ct.GetMetadata()
			k := md.GetKind() // *metadatav3.MetadataKind — the kind-getters live HERE, not on md (V1 SEVERE)
			switch {
			case k == nil:
				return nil, fmt.Errorf("tracing: custom_tags metadata tag %q kind required", tag)
			case k.GetRequest() != nil:
				mk := md.GetMetadataKey()
				if mk.GetKey() == "" {
					return nil, fmt.Errorf("tracing: custom_tags metadata tag %q empty namespace", tag)
				}
				if len(mk.GetPath()) == 0 {
					return nil, fmt.Errorf("tracing: custom_tags metadata tag %q empty path", tag)
				}
				path := make([]string, 0, len(mk.GetPath()))
				for _, seg := range mk.GetPath() {
					if seg.GetKey() == "" {
						return nil, fmt.Errorf("tracing: custom_tags metadata tag %q empty path segment", tag)
					}
					path = append(path, seg.GetKey())
				}
				dv := md.GetDefaultValue()
				spec = CustomTagSpec{Key: tag, Kind: kindMetadata, MetaNamespace: mk.GetKey(), MetaPath: path, DefaultValue: dv, HasDefault: dv != ""}
			case k.GetRoute() != nil:
				return nil, fmt.Errorf("tracing: custom_tags metadata tag %q route kind unsupported", tag)
			case k.GetCluster() != nil:
				return nil, fmt.Errorf("tracing: custom_tags metadata tag %q cluster kind unsupported", tag)
			case k.GetHost() != nil:
				return nil, fmt.Errorf("tracing: custom_tags metadata tag %q host kind unsupported", tag)
			default:
				return nil, fmt.Errorf("tracing: custom_tags metadata tag %q kind required", tag)
			}
		default:
			return nil, fmt.Errorf("tracing: custom_tags tag %q missing type", tag)
		}
		if _, dup := seen[tag]; dup {
			continue // first-wins: drop a later same-key tag (validated above)
		}
		seen[tag] = struct{}{}
		out = append(out, spec)
	}
	return out, nil
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
