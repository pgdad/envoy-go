package boot

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pgdad/envoy-go/internal/xds"
)

// This file exercises boot.NewValidateSDSProvider (phase 86, ADR-0308): the
// --mode validate counterpart to NewSDSProvider that runs the ENTIRE boot
// pre-scan via the newSDSProviderAndClient split (parity by CODE-PATH — every
// present and future pre-scan arm is inherited by construction) but never
// dials, closing the never-dialed lazy client and returning the no-fetch
// sentinel instead of a live provider.
//
// Every negative wantSub below is a PRE-EXISTING boot-parity string (frozen
// roster, PLAN §5) — this task adds ZERO new reject strings. Assertions use
// strings.Contains rather than equality because these strings are, at other
// call sites (the listener build), wrapped with `listener: %q:
// filter_chains[%d]:` — the substring survives that wrap even though these
// particular tests call NewValidateSDSProvider directly (pre-wrap).

// yamlTwoSDSListenersTemplate builds a bootstrap with TWO SEPARATE listeners,
// each carrying its OWN single SDS-bound downstream TLS context (cert-only).
// This is deliberately NOT one context with two
// tls_certificate_sds_secret_configs entries — that shape trips
// xds.ParseSDSConfig's per-context "multiple tls_certificate_sds_secret_configs
// unsupported (MVP takes one, got 2)" instead, a different string (Task 0
// census). Two positions is what actually reaches the pre-scan's own
// seen>1 guard in boot.go.
const yamlTwoSDSListenersTemplate = `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_tls_a
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
          transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              common_tls_context:
                tls_certificate_sds_secret_configs:
                  - name: server_cert_a
                    sds_config: %[1]s
    - name: l_tls_b
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
          transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              common_tls_context:
                tls_certificate_sds_secret_configs:
                  - name: server_cert_b
                    sds_config: %[1]s
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
    - name: sds_cluster
      type: STATIC
      connect_timeout: 1s
      typed_extension_protocol_options:
        envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
          "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
          explicit_http_config:
            http2_protocol_options: {}
      load_assignment:
        cluster_name: sds_cluster
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 1 }
`

// sdsListenerYAMLNoH2Template mirrors sdsListenerYAMLTemplate (boot_test.go)
// exactly, EXCEPT sds_cluster carries no typed_extension_protocol_options —
// so cluster.UseH2() is false and grpcclient.Dialer.DialContext's synchronous
// H2 PARSE-REJECT fires (n7: absent from every prior enumeration, Task 0
// census; SPEC §2 Q2).
const sdsListenerYAMLNoH2Template = `
node: { id: "%s", cluster: "%s" }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_tls
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
          transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              common_tls_context:
                tls_certificate_sds_secret_configs:
                  - name: server_cert
                    sds_config: %s
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
    - name: sds_cluster
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: sds_cluster
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: %d }
`

// deltaGRPCSdsConfigFlow is a well-formed api_config_source EXCEPT its
// api_type is DELTA_GRPC — xds.ParseSDSConfig arm rejects this (only
// GRPC / State-of-the-World is supported).
const deltaGRPCSdsConfigFlow = `{resource_api_version: V3, api_config_source: {api_type: DELTA_GRPC, transport_api_version: V3, grpc_services: [{envoy_grpc: {cluster_name: sds_cluster}}]}}`

// missingClusterSdsConfigFlow is a well-formed api_config_source pointed at a
// cluster_name that has NO matching static cluster resource — the dialer's
// synchronous `mgr.Get` PARSE-REJECT fires (n4: the misleading-but-real
// "dial cluster" prefix, with NO dial actually happening; SPEC §2's
// sub-finding — quoted verbatim, not "fixed").
const missingClusterSdsConfigFlow = `{resource_api_version: V3, api_config_source: {api_type: GRPC, transport_api_version: V3, grpc_services: [{envoy_grpc: {cluster_name: missing_cluster}}]}}`

// ctcBadSecretNameFlow carries a hyphenated secret name ("server-cert") —
// stats.IsValidName rejects the hyphen in the assembled sds.<name>.<suffix>
// stat name (validateSDSSecretName's DEPARTURE, phase 80; boot_test.go
// documents the reference accepts this).
const ctcBadSecretNameFlow = `{tls_certificate_sds_secret_configs: [{name: server-cert, sds_config: %s}]}`

// TestNewValidateSDSProvider_BootParityNegatives is the six build-time
// negative arms (PLAN §5's frozen roster): each must return the SAME string
// the ordinary boot path (NewSDSProvider) returns for the identical shape —
// parity by CODE-PATH reuse via newSDSProviderAndClient, not a re-derivation.
func TestNewValidateSDSProvider_BootParityNegatives(t *testing.T) {
	yamlArmANoNode := fmt.Sprintf(sdsListenerYAMLTemplate, "", "", grpcSdsConfigFlow, 1)
	yamlTwoSDS := fmt.Sprintf(yamlTwoSDSListenersTemplate, grpcSdsConfigFlow)
	yamlDeltaGRPC := fmt.Sprintf(sdsListenerYAMLTemplate, "test-node", "test-cluster", deltaGRPCSdsConfigFlow, 1)
	yamlUnknownCluster := fmt.Sprintf(sdsListenerYAMLTemplate, "test-node", "test-cluster", missingClusterSdsConfigFlow, 1)
	badNameCTC := fmt.Sprintf(ctcBadSecretNameFlow, grpcSdsConfigFlow)
	yamlBadSecretName := fmt.Sprintf(sdsListenerCTCYAMLTemplate, "test-node", "test-cluster", badNameCTC, 1)
	yamlNoH2 := fmt.Sprintf(sdsListenerYAMLNoH2Template, "test-node", "test-cluster", grpcSdsConfigFlow, 1)

	cases := []struct{ name, yaml, wantSub string }{
		{"n1-node-absent", yamlArmANoNode, "xds: sds: node.id and node.cluster are required for SDS"},
		{"n2-two-positions", yamlTwoSDS, "multiple SDS-bound downstream TLS contexts unsupported"},
		{"n3-delta-grpc", yamlDeltaGRPC, "DELTA_GRPC api_type unsupported"},
		{"n4-unknown-cluster", yamlUnknownCluster, `dial cluster "missing_cluster": grpcclient: dial "missing_cluster": unknown cluster`},
		{"n6-bad-name", yamlBadSecretName, "invalid secret name"},
		{"n7-no-h2", yamlNoH2, "cluster does not have http2_protocol_options{} set"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bs, dialer := loadSDSBootstrapAndDialer(t, tc.yaml)
			_, err := NewValidateSDSProvider(dialer, bs, t.TempDir(), bs.Stats)
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("NewValidateSDSProvider(%s) = %v, want substring %q", tc.name, err, tc.wantSub)
			}
		})
	}
}

// TestNewValidateSDSProvider_NoSDSReturnsNilNil: a bootstrap with no
// SDS-bound downstream TLS context anywhere (seen==0) must return (nil, nil)
// exactly like NewSDSProvider — the nil provider threads harmlessly.
func TestNewValidateSDSProvider_NoSDSReturnsNilNil(t *testing.T) {
	bs, dialer := loadSDSBootstrapAndDialer(t, validYAML)
	provider, err := NewValidateSDSProvider(dialer, bs, t.TempDir(), bs.Stats)
	if err != nil {
		t.Errorf("NewValidateSDSProvider: got error %v, want nil", err)
	}
	if provider != nil {
		t.Errorf("NewValidateSDSProvider: got non-nil provider, want nil")
	}
}

// TestNewValidateSDSProvider_ArmPositiveReturnsSentinel: a well-formed
// cert-only SDS shape (arm-A) must return the NON-NIL no-fetch sentinel — not
// a live xds.Provider, and never provider == nil (the discriminator is the
// sentinel TYPE, D-86-MECH).
func TestNewValidateSDSProvider_ArmPositiveReturnsSentinel(t *testing.T) {
	ctc := fmt.Sprintf(ctcCertOnlySDSFlow, grpcSdsConfigFlow)
	yaml := fmt.Sprintf(sdsListenerCTCYAMLTemplate, "test-node", "test-cluster", ctc, 1)
	bs, dialer := loadSDSBootstrapAndDialer(t, yaml)

	provider, err := NewValidateSDSProvider(dialer, bs, t.TempDir(), bs.Stats)
	if err != nil {
		t.Fatalf("NewValidateSDSProvider: got error %v, want nil", err)
	}
	if provider == nil {
		t.Fatal("NewValidateSDSProvider: got nil provider, want the no-fetch sentinel")
	}
	if !xds.IsNoFetch(provider) {
		t.Error("NewValidateSDSProvider: provider is not the no-fetch sentinel (xds.IsNoFetch == false)")
	}
}

// TestNewValidateSDSProvider_NC7_PureInlineCVC_RetainedRejectSurvives is NC-7
// (PLAN §4): a bootstrap whose only downstream TLS context is a PURE-INLINE
// combined_validation_context (no SDS half — seen stays 0 per the pre-scan's
// E2/D2 skip) must make NewValidateSDSProvider return (nil, nil) exactly like
// the ordinary boot path, AND the downstream build (main.go's real
// seen==0-then-Construct shape) must still hit the retained phase-03 CVC
// reject — proving this task's split does not accidentally suppress or
// reroute that unrelated, pre-existing reject.
func TestNewValidateSDSProvider_NC7_PureInlineCVC_RetainedRejectSurvives(t *testing.T) {
	yaml := fmt.Sprintf(sdsListenerCTCYAMLTemplate, "test-node", "test-cluster", ctcCVCPureInlineFlow, 1)

	bs, dialer := loadSDSBootstrapAndDialer(t, yaml)
	provider, err := NewValidateSDSProvider(dialer, bs, t.TempDir(), bs.Stats)
	if err != nil {
		t.Errorf("NewValidateSDSProvider: got error %v, want nil", err)
	}
	if provider != nil {
		t.Errorf("NewValidateSDSProvider: got non-nil provider, want nil — a pure-inline CVC contributes no secret")
	}

	_, buildErr := mustConstruct(t, yaml)
	if buildErr == nil {
		t.Fatal("Construct: want the retained phase-03 CVC reject, got nil")
	}
	const wantCVC = "combined_validation_context is not supported in phase 03"
	if !strings.Contains(buildErr.Error(), wantCVC) {
		t.Errorf("Construct error = %q, want substring %q", buildErr.Error(), wantCVC)
	}
}
