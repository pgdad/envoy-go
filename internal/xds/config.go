package xds

import (
	"fmt"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
)

// defaultInitialFetchTimeout mirrors the reference's SDS initial_fetch_timeout
// default (15s, SPEC §3.4 / §11 Arm B) when the config leaves it unset.
const defaultInitialFetchTimeout = 15 * time.Second

// ParseSDSConfig validates a downstream tls_certificate_sds_secret_configs list
// and extracts the (secretName, clusterName, initial_fetch_timeout) triple the
// SDS provider needs. MVP: exactly ONE config (a fallback list is a later row).
// Every reject is ADR-0080-distinct and "xds: sds:"-prefixed — the reference
// SUPPORTS all these forms, so they are documented envoy-go-strict DEPARTURES.
// This function imports NEITHER internal/grpcclient NOR internal/cluster NOR
// internal/tls (the ADR-0278 cycle guard): it operates on already-typed protos.
func ParseSDSConfig(configs []*tlsv3.SdsSecretConfig) (secretName, clusterName string, timeout time.Duration, err error) {
	if len(configs) != 1 {
		return "", "", 0, fmt.Errorf("xds: sds: multiple tls_certificate_sds_secret_configs unsupported (MVP takes one, got %d)", len(configs))
	}
	sc := configs[0]
	name := sc.GetName()
	if name == "" {
		return "", "", 0, fmt.Errorf("xds: sds: SdsSecretConfig name is required")
	}
	cs := sc.GetSdsConfig()
	if cs == nil {
		return "", "", 0, fmt.Errorf("xds: sds: SdsSecretConfig sds_config is required")
	}
	if cs.GetAds() != nil {
		return "", "", 0, fmt.Errorf("xds: sds: ads-sourced ConfigSource unsupported (only api_config_source)")
	}
	if cs.GetSelf() != nil {
		return "", "", 0, fmt.Errorf("xds: sds: self-sourced ConfigSource unsupported (only api_config_source)")
	}
	acs := cs.GetApiConfigSource()
	if acs == nil {
		return "", "", 0, fmt.Errorf("xds: sds: ConfigSource must be an api_config_source")
	}
	if cs.GetResourceApiVersion() != corev3.ApiVersion_V3 {
		return "", "", 0, fmt.Errorf("xds: sds: resource_api_version must be V3")
	}
	switch acs.GetApiType() {
	case corev3.ApiConfigSource_GRPC:
		// ok
	case corev3.ApiConfigSource_DELTA_GRPC:
		return "", "", 0, fmt.Errorf("xds: sds: DELTA_GRPC api_type unsupported (only GRPC / State-of-the-World)")
	default:
		return "", "", 0, fmt.Errorf("xds: sds: api_type must be GRPC")
	}
	if acs.GetTransportApiVersion() != corev3.ApiVersion_V3 {
		return "", "", 0, fmt.Errorf("xds: sds: transport_api_version must be V3")
	}
	gs := acs.GetGrpcServices()
	if len(gs) != 1 {
		return "", "", 0, fmt.Errorf("xds: sds: exactly one grpc_service required (got %d)", len(gs))
	}
	if gs[0].GetGoogleGrpc() != nil {
		return "", "", 0, fmt.Errorf("xds: sds: google_grpc transport unsupported (only envoy_grpc)")
	}
	eg := gs[0].GetEnvoyGrpc()
	if eg == nil {
		return "", "", 0, fmt.Errorf("xds: sds: grpc_service must be envoy_grpc")
	}
	cluster := eg.GetClusterName()
	if cluster == "" {
		return "", "", 0, fmt.Errorf("xds: sds: envoy_grpc cluster_name is required")
	}
	timeout = defaultInitialFetchTimeout
	if d := cs.GetInitialFetchTimeout(); d != nil {
		timeout = d.AsDuration()
	}
	return name, cluster, timeout, nil
}
