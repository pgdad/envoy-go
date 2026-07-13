package xds

import (
	"strings"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/types/known/durationpb"
)

// sdsCfg builds a valid *tlsv3.SdsSecretConfig (api_config_source, GRPC, V3,
// envoy_grpc -> cluster, resource_api_version V3) that mut can corrupt to
// exercise the reject arms.
func sdsCfg(name, cluster string, mut ...func(*corev3.ConfigSource)) *tlsv3.SdsSecretConfig {
	cs := &corev3.ConfigSource{
		ConfigSourceSpecifier: &corev3.ConfigSource_ApiConfigSource{
			ApiConfigSource: &corev3.ApiConfigSource{
				ApiType:             corev3.ApiConfigSource_GRPC,
				TransportApiVersion: corev3.ApiVersion_V3,
				GrpcServices: []*corev3.GrpcService{
					{
						TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
							EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{
								ClusterName: cluster,
							},
						},
					},
				},
			},
		},
		ResourceApiVersion: corev3.ApiVersion_V3,
	}
	for _, m := range mut {
		m(cs)
	}
	return &tlsv3.SdsSecretConfig{
		Name:      name,
		SdsConfig: cs,
	}
}

func TestParseSDSConfig(t *testing.T) {
	cases := []struct {
		name        string
		configs     []*tlsv3.SdsSecretConfig
		wantSecret  string
		wantCluster string
		wantTimeout time.Duration
		wantErrSub  string
	}{
		{
			name:        "accept",
			configs:     []*tlsv3.SdsSecretConfig{sdsCfg("server_cert", "sds_cluster")},
			wantSecret:  "server_cert",
			wantCluster: "sds_cluster",
			wantTimeout: 15 * time.Second,
		},
		{
			name: "accept with explicit timeout",
			configs: []*tlsv3.SdsSecretConfig{sdsCfg("server_cert", "sds_cluster", func(cs *corev3.ConfigSource) {
				cs.InitialFetchTimeout = durationpb.New(3 * time.Second)
			})},
			wantSecret:  "server_cert",
			wantCluster: "sds_cluster",
			wantTimeout: 3 * time.Second,
		},
		{
			name:       "arm 8 empty name",
			configs:    []*tlsv3.SdsSecretConfig{sdsCfg("", "sds_cluster")},
			wantErrSub: "name is required",
		},
		{
			name: "arm 9 missing sds_config",
			configs: []*tlsv3.SdsSecretConfig{
				{Name: "server_cert", SdsConfig: nil},
			},
			wantErrSub: "sds_config is required",
		},
		{
			name: "arm 1 ads",
			configs: []*tlsv3.SdsSecretConfig{sdsCfg("server_cert", "sds_cluster", func(cs *corev3.ConfigSource) {
				cs.ConfigSourceSpecifier = &corev3.ConfigSource_Ads{Ads: &corev3.AggregatedConfigSource{}}
			})},
			wantErrSub: "ads-sourced",
		},
		{
			name: "arm 2 self",
			configs: []*tlsv3.SdsSecretConfig{sdsCfg("server_cert", "sds_cluster", func(cs *corev3.ConfigSource) {
				cs.ConfigSourceSpecifier = &corev3.ConfigSource_Self{Self: &corev3.SelfConfigSource{}}
			})},
			wantErrSub: "self-sourced",
		},
		{
			name: "arm 3 DELTA_GRPC",
			configs: []*tlsv3.SdsSecretConfig{sdsCfg("server_cert", "sds_cluster", func(cs *corev3.ConfigSource) {
				cs.GetApiConfigSource().ApiType = corev3.ApiConfigSource_DELTA_GRPC
			})},
			wantErrSub: "DELTA_GRPC",
		},
		{
			name: "arm 4 google_grpc",
			configs: []*tlsv3.SdsSecretConfig{sdsCfg("server_cert", "sds_cluster", func(cs *corev3.ConfigSource) {
				cs.GetApiConfigSource().GrpcServices = []*corev3.GrpcService{
					{
						TargetSpecifier: &corev3.GrpcService_GoogleGrpc_{
							GoogleGrpc: &corev3.GrpcService_GoogleGrpc{
								TargetUri:  "sds.example.com:443",
								StatPrefix: "sds",
							},
						},
					},
				}
			})},
			wantErrSub: "google_grpc",
		},
		{
			name: "non-api_config_source",
			configs: []*tlsv3.SdsSecretConfig{sdsCfg("server_cert", "sds_cluster", func(cs *corev3.ConfigSource) {
				cs.ConfigSourceSpecifier = &corev3.ConfigSource_Path{Path: "/etc/envoy/sds.yaml"}
			})},
			wantErrSub: "api_config_source",
		},
		{
			name: "non-V3 resource",
			configs: []*tlsv3.SdsSecretConfig{sdsCfg("server_cert", "sds_cluster", func(cs *corev3.ConfigSource) {
				cs.ResourceApiVersion = corev3.ApiVersion_V2 //nolint:staticcheck // SA1019: intentional reject-arm probe of the deprecated V2 resource_api_version.
			})},
			wantErrSub: "resource_api_version",
		},
		{
			name: "non-V3 transport",
			configs: []*tlsv3.SdsSecretConfig{sdsCfg("server_cert", "sds_cluster", func(cs *corev3.ConfigSource) {
				cs.GetApiConfigSource().TransportApiVersion = corev3.ApiVersion_V2 //nolint:staticcheck // SA1019: intentional reject-arm probe of the deprecated V2 transport_api_version.
			})},
			wantErrSub: "transport_api_version",
		},
		{
			name:       "empty cluster_name",
			configs:    []*tlsv3.SdsSecretConfig{sdsCfg("server_cert", "")},
			wantErrSub: "cluster_name is required",
		},
		{
			name:       "multi-cap",
			configs:    []*tlsv3.SdsSecretConfig{sdsCfg("server_cert", "sds_cluster"), sdsCfg("other_cert", "sds_cluster")},
			wantErrSub: "multiple",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			secret, cluster, timeout, err := ParseSDSConfig(tc.configs)
			if tc.wantErrSub != "" {
				if err == nil {
					t.Errorf("%s: expected error containing %q, got nil", tc.name, tc.wantErrSub)
					return
				}
				if !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Errorf("%s: error = %q, want substring %q", tc.name, err.Error(), tc.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Errorf("%s: unexpected error: %v", tc.name, err)
				return
			}
			if secret != tc.wantSecret {
				t.Errorf("%s: secret = %q, want %q", tc.name, secret, tc.wantSecret)
			}
			if cluster != tc.wantCluster {
				t.Errorf("%s: cluster = %q, want %q", tc.name, cluster, tc.wantCluster)
			}
			if timeout != tc.wantTimeout {
				t.Errorf("%s: timeout = %v, want %v", tc.name, timeout, tc.wantTimeout)
			}
		})
	}
}
