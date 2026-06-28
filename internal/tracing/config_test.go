package tracing

import (
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tracev3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	typetracingv3 "github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// defaultConfig returns the reference-default TracingConfig (all three sampling
// knobs at 100%). This is a placeholder until Task 5 adds the proto->config
// parse; the zero-knob distinction (an EXPLICIT 0.0 is a valid 0% sample,
// distinct from ABSENT => 100) is applied at parse time in Task 5, not here.
func defaultConfig() *TracingConfig {
	return &TracingConfig{
		ClientSampling:  100,
		RandomSampling:  100,
		OverallSampling: 100,
	}
}

func TestDefaultConfigSamplingKnobs(t *testing.T) {
	c := defaultConfig()
	if c.ClientSampling != 100 {
		t.Errorf("ClientSampling = %v, want 100", c.ClientSampling)
	}
	if c.RandomSampling != 100 {
		t.Errorf("RandomSampling = %v, want 100", c.RandomSampling)
	}
	if c.OverallSampling != 100 {
		t.Errorf("OverallSampling = %v, want 100", c.OverallSampling)
	}
}

// TestExplicitZeroSamplingIsDistinctFromAbsent documents the zero-knob handling:
// a value of 0.0 is a VALID 0% sample and is preserved by the struct (it is NOT
// coerced to the absent => 100 default). The ABSENT => 100 coercion is applied
// at parse time (Task 5); the struct itself stores whatever it is given.
func TestExplicitZeroSamplingIsDistinctFromAbsent(t *testing.T) {
	c := &TracingConfig{ClientSampling: 0, RandomSampling: 0, OverallSampling: 0}
	if c.ClientSampling != 0 || c.RandomSampling != 0 || c.OverallSampling != 0 {
		t.Fatalf("explicit 0 sampling not preserved: %+v", c)
	}
}

func TestExporterFieldsUnusedAt46_1a(t *testing.T) {
	c := &TracingConfig{ServiceName: "svc", ClusterName: "cl"}
	if c.ServiceName != "svc" {
		t.Errorf("ServiceName = %q, want %q", c.ServiceName, "svc")
	}
	if c.ClusterName != "cl" {
		t.Errorf("ClusterName = %q, want %q", c.ClusterName, "cl")
	}
}

// otelProvider builds a *hcmv3.HttpConnectionManager_Tracing whose provider
// carries the given OpenTelemetryConfig marshaled into the typed_config Any.
func otelProvider(t *testing.T, otel *tracev3.OpenTelemetryConfig) *hcmv3.HttpConnectionManager_Tracing {
	t.Helper()
	any, err := anypb.New(otel)
	if err != nil {
		t.Fatalf("anypb.New(otel): %v", err)
	}
	return &hcmv3.HttpConnectionManager_Tracing{
		Provider: &tracev3.Tracing_Http{
			Name:       "envoy.tracers.opentelemetry",
			ConfigType: &tracev3.Tracing_Http_TypedConfig{TypedConfig: any},
		},
	}
}

func envoyGrpcOTel(cluster, service string) *tracev3.OpenTelemetryConfig {
	return &tracev3.OpenTelemetryConfig{
		GrpcService: &corev3.GrpcService{
			TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: cluster},
			},
		},
		ServiceName: service,
	}
}

func TestNewConfigNil(t *testing.T) {
	got, err := NewConfig(nil)
	if err != nil {
		t.Fatalf("NewConfig(nil) err = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("NewConfig(nil) = %+v, want nil", got)
	}
}

func TestNewConfigAcceptMinimal(t *testing.T) {
	tr := otelProvider(t, envoyGrpcOTel("c", "svc"))
	got, err := NewConfig(tr)
	if err != nil {
		t.Fatalf("NewConfig err = %v, want nil", err)
	}
	want := &TracingConfig{ClientSampling: 100, RandomSampling: 100, OverallSampling: 100, ServiceName: "svc", ClusterName: "c"}
	if *got != *want {
		t.Fatalf("NewConfig = %+v, want %+v", got, want)
	}
}

func TestNewConfigAcceptKnobs(t *testing.T) {
	tr := otelProvider(t, envoyGrpcOTel("c", "svc"))
	tr.RandomSampling = &typev3.Percent{Value: 50}
	tr.ClientSampling = &typev3.Percent{Value: 0}
	tr.OverallSampling = &typev3.Percent{Value: 100}
	got, err := NewConfig(tr)
	if err != nil {
		t.Fatalf("NewConfig err = %v, want nil", err)
	}
	if got.RandomSampling != 50 {
		t.Errorf("RandomSampling = %v, want 50", got.RandomSampling)
	}
	if got.ClientSampling != 0 {
		t.Errorf("ClientSampling = %v, want 0 (explicit zero preserved)", got.ClientSampling)
	}
	if got.OverallSampling != 100 {
		t.Errorf("OverallSampling = %v, want 100", got.OverallSampling)
	}
}

// zipkinProvider builds a *hcmv3.HttpConnectionManager_Tracing whose provider
// carries the given ZipkinConfig marshaled into the typed_config Any.
func zipkinProvider(t *testing.T, zk *tracev3.ZipkinConfig) *hcmv3.HttpConnectionManager_Tracing {
	t.Helper()
	any, err := anypb.New(zk)
	if err != nil {
		t.Fatalf("anypb.New(zipkin): %v", err)
	}
	return &hcmv3.HttpConnectionManager_Tracing{
		Provider: &tracev3.Tracing_Http{
			Name:       "envoy.tracers.zipkin",
			ConfigType: &tracev3.Tracing_Http_TypedConfig{TypedConfig: any},
		},
	}
}

func minimalZipkin() *tracev3.ZipkinConfig {
	return &tracev3.ZipkinConfig{
		CollectorCluster:         "zk",
		CollectorEndpoint:        "/api/v2/spans",
		CollectorEndpointVersion: tracev3.ZipkinConfig_HTTP_JSON,
	}
}

func TestNewConfigAcceptZipkinMinimal(t *testing.T) {
	tr := zipkinProvider(t, minimalZipkin())
	got, err := NewConfig(tr)
	if err != nil {
		t.Fatalf("NewConfig err = %v, want nil", err)
	}
	if got.Provider != ProviderZipkin {
		t.Fatalf("Provider = %v, want ProviderZipkin", got.Provider)
	}
	if got.ClusterName != "zk" {
		t.Errorf("ClusterName = %q, want %q", got.ClusterName, "zk")
	}
	if got.Zipkin == nil {
		t.Fatalf("Zipkin = nil, want non-nil")
	}
	if got.Zipkin.CollectorEndpoint != "/api/v2/spans" {
		t.Errorf("Zipkin.CollectorEndpoint = %q, want %q", got.Zipkin.CollectorEndpoint, "/api/v2/spans")
	}
	if !got.Zipkin.SharedSpanContext {
		t.Errorf("Zipkin.SharedSpanContext = false, want true (absent => default true)")
	}
	if got.Zipkin.TraceID128Bit {
		t.Errorf("Zipkin.TraceID128Bit = true, want false")
	}
	if got.ClientSampling != 100 || got.RandomSampling != 100 || got.OverallSampling != 100 {
		t.Errorf("sampling = %v/%v/%v, want 100/100/100", got.ClientSampling, got.RandomSampling, got.OverallSampling)
	}
	if got.ServiceName != "" {
		t.Errorf("ServiceName = %q, want empty (Zipkin has no service_name)", got.ServiceName)
	}
}

func TestNewConfigAcceptZipkinKnobs(t *testing.T) {
	zk := minimalZipkin()
	zk.TraceId_128Bit = true
	zk.SharedSpanContext = wrapperspb.Bool(false)
	zk.CollectorHostname = "h"
	got, err := NewConfig(zipkinProvider(t, zk))
	if err != nil {
		t.Fatalf("NewConfig err = %v, want nil", err)
	}
	if !got.Zipkin.TraceID128Bit {
		t.Errorf("Zipkin.TraceID128Bit = false, want true")
	}
	if got.Zipkin.SharedSpanContext {
		t.Errorf("Zipkin.SharedSpanContext = true, want false (explicit false)")
	}
	if got.Zipkin.CollectorHostname != "h" {
		t.Errorf("Zipkin.CollectorHostname = %q, want %q", got.Zipkin.CollectorHostname, "h")
	}
}

func TestNewConfigOTelProviderUnchanged(t *testing.T) {
	tr := otelProvider(t, envoyGrpcOTel("c", "svc"))
	got, err := NewConfig(tr)
	if err != nil {
		t.Fatalf("NewConfig err = %v, want nil", err)
	}
	if got.Provider != ProviderOTel {
		t.Errorf("Provider = %v, want ProviderOTel", got.Provider)
	}
	if got.Zipkin != nil {
		t.Errorf("Zipkin = %+v, want nil for the OTel arm", got.Zipkin)
	}
}

func TestNewConfigRejectZipkinArms(t *testing.T) {
	tests := []struct {
		name string
		mut  func() *tracev3.ZipkinConfig
	}{
		{
			name: "collector_endpoint_version-HTTP_PROTO",
			mut: func() *tracev3.ZipkinConfig {
				zk := minimalZipkin()
				zk.CollectorEndpointVersion = tracev3.ZipkinConfig_HTTP_PROTO
				return zk
			},
		},
		{
			name: "collector_endpoint_version-GRPC",
			mut: func() *tracev3.ZipkinConfig {
				zk := minimalZipkin()
				zk.CollectorEndpointVersion = tracev3.ZipkinConfig_GRPC
				return zk
			},
		},
		{
			name: "collector_endpoint_version-DEPRECATED",
			mut: func() *tracev3.ZipkinConfig {
				zk := minimalZipkin()
				zk.CollectorEndpointVersion = tracev3.ZipkinConfig_DEPRECATED_AND_UNAVAILABLE_DO_NOT_USE //nolint:staticcheck // SA1019: intentional reject-arm probe of the deprecated enum value.
				return zk
			},
		},
		{
			name: "split_spans_for_request",
			mut: func() *tracev3.ZipkinConfig {
				zk := minimalZipkin()
				zk.SplitSpansForRequest = true //nolint:staticcheck // SA1019: intentional reject-arm probe of the deprecated field.
				return zk
			},
		},
		{
			name: "empty-collector_cluster",
			mut: func() *tracev3.ZipkinConfig {
				zk := minimalZipkin()
				zk.CollectorCluster = ""
				return zk
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewConfig(zipkinProvider(t, tc.mut()))
			if err == nil {
				t.Fatalf("NewConfig(%s) err = nil, want reject error; got %+v", tc.name, got)
			}
			if got != nil {
				t.Fatalf("NewConfig(%s) returned non-nil config %+v on reject", tc.name, got)
			}
		})
	}
}

func TestNewConfigRejectArms(t *testing.T) {
	tests := []struct {
		name string
		mut  func(t *testing.T) *hcmv3.HttpConnectionManager_Tracing
	}{
		{
			name: "non-otel-provider",
			mut: func(t *testing.T) *hcmv3.HttpConnectionManager_Tracing {
				any, err := anypb.New(&tracev3.ZipkinConfig{CollectorCluster: "z"})
				if err != nil {
					t.Fatalf("anypb.New(zipkin): %v", err)
				}
				return &hcmv3.HttpConnectionManager_Tracing{
					Provider: &tracev3.Tracing_Http{
						Name:       "envoy.tracers.zipkin",
						ConfigType: &tracev3.Tracing_Http_TypedConfig{TypedConfig: any},
					},
				}
			},
		},
		{
			name: "empty-cluster",
			mut: func(t *testing.T) *hcmv3.HttpConnectionManager_Tracing {
				return otelProvider(t, envoyGrpcOTel("", "svc"))
			},
		},
		{
			name: "google_grpc",
			mut: func(t *testing.T) *hcmv3.HttpConnectionManager_Tracing {
				otel := &tracev3.OpenTelemetryConfig{
					GrpcService: &corev3.GrpcService{
						TargetSpecifier: &corev3.GrpcService_GoogleGrpc_{
							GoogleGrpc: &corev3.GrpcService_GoogleGrpc{TargetUri: "u", StatPrefix: "p"},
						},
					},
				}
				return otelProvider(t, otel)
			},
		},
		{
			name: "custom_tags",
			mut: func(t *testing.T) *hcmv3.HttpConnectionManager_Tracing {
				tr := otelProvider(t, envoyGrpcOTel("c", "svc"))
				tr.CustomTags = []*typetracingv3.CustomTag{{Tag: "x"}}
				return tr
			},
		},
		{
			name: "verbose",
			mut: func(t *testing.T) *hcmv3.HttpConnectionManager_Tracing {
				tr := otelProvider(t, envoyGrpcOTel("c", "svc"))
				tr.Verbose = true
				return tr
			},
		},
		{
			name: "max_path_tag_length",
			mut: func(t *testing.T) *hcmv3.HttpConnectionManager_Tracing {
				tr := otelProvider(t, envoyGrpcOTel("c", "svc"))
				tr.MaxPathTagLength = wrapperspb.UInt32(128)
				return tr
			},
		},
		{
			name: "spawn_upstream_span",
			mut: func(t *testing.T) *hcmv3.HttpConnectionManager_Tracing {
				tr := otelProvider(t, envoyGrpcOTel("c", "svc"))
				tr.SpawnUpstreamSpan = wrapperspb.Bool(false)
				return tr
			},
		},
		{
			name: "http_service",
			mut: func(t *testing.T) *hcmv3.HttpConnectionManager_Tracing {
				otel := envoyGrpcOTel("c", "svc")
				otel.HttpService = &corev3.HttpService{}
				return otelProvider(t, otel)
			},
		},
		{
			name: "resource_detectors",
			mut: func(t *testing.T) *hcmv3.HttpConnectionManager_Tracing {
				otel := envoyGrpcOTel("c", "svc")
				otel.ResourceDetectors = []*corev3.TypedExtensionConfig{{Name: "d"}}
				return otelProvider(t, otel)
			},
		},
		{
			name: "sampler",
			mut: func(t *testing.T) *hcmv3.HttpConnectionManager_Tracing {
				otel := envoyGrpcOTel("c", "svc")
				otel.Sampler = &corev3.TypedExtensionConfig{Name: "s"}
				return otelProvider(t, otel)
			},
		},
		{
			name: "nil-provider",
			mut: func(t *testing.T) *hcmv3.HttpConnectionManager_Tracing {
				return &hcmv3.HttpConnectionManager_Tracing{}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewConfig(tc.mut(t))
			if err == nil {
				t.Fatalf("NewConfig(%s) err = nil, want reject error; got %+v", tc.name, got)
			}
			if got != nil {
				t.Fatalf("NewConfig(%s) returned non-nil config %+v on reject", tc.name, got)
			}
		})
	}
}
