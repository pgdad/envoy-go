package boot

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pgdad/envoy-go/internal/bootstrap"
	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/drain"
	"github.com/pgdad/envoy-go/internal/grpcclient"
	"github.com/pgdad/envoy-go/internal/httpclient"
	"github.com/pgdad/envoy-go/internal/listener"
	"github.com/pgdad/envoy-go/test/helpers/sdsserver"
)

const validYAML = `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
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
`

func mustConstruct(t *testing.T, yaml string) (*listener.Manager, error) {
	t.Helper()
	bs, err := bootstrap.Load(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("bootstrap.Load: %v", err)
	}
	cm, err := cluster.NewManagerWithBaseDir(bs.Proto, t.TempDir(), bs.Stats)
	if err != nil {
		t.Fatalf("cluster.NewManagerWithBaseDir: %v", err)
	}
	dm := drain.New(30 * time.Second)
	httpClient := httpclient.New(httpclient.Options{Timeout: 30 * time.Second})
	dialer := grpcclient.New(cm)
	tracingProvider := NewTracingProvider(dialer, httpClient, cm, bs.Stats)
	return Construct(bs, cm, t.TempDir(), false, nil, dm, httpClient, tracingProvider, nil)
}

func TestConstruct_ValidBootstrap_ReturnsNilError(t *testing.T) {
	lm, err := mustConstruct(t, validYAML)
	if err != nil {
		t.Fatalf("Construct: got error %v, want nil", err)
	}
	if lm == nil {
		t.Fatal("Construct: got nil *listener.Manager on success, want non-nil")
	}
}

func TestConstruct_LuaCompileFailure_WrapsWithScriptLoadErrorPrefix(t *testing.T) {
	luaYAML := `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: hcm_local
                route_config:
                  name: rc
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_echo }
                http_filters:
                  - name: envoy.filters.http.lua
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua
                      default_source_code:
                        inline_string: "this is not ((( valid lua syntax"
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
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
`
	_, err := mustConstruct(t, luaYAML)
	if err == nil {
		t.Fatal("Construct: want error for invalid Lua syntax, got nil")
	}
	if !strings.Contains(err.Error(), "script load error: ") {
		t.Errorf("error should contain the script-load-error wrap prefix: %q", err.Error())
	}
}

// sdsListenerYAMLTemplate builds a bootstrap with one listener whose sole
// filter_chain carries a downstream TLS transport_socket bound to a single
// tls_certificate_sds_secret_configs entry named "server_cert" pointed at a
// static "sds_cluster". %s/%s = node.id/node.cluster (may be ""); %s =
// the flow-style sds_config body (varies per test to exercise the ADS-reject
// vs. valid-GRPC arms); %d = sds_cluster's single endpoint port.
const sdsListenerYAMLTemplate = `
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
                    socket_address: { address: 127.0.0.1, port_value: %d }
`

// grpcSdsConfigFlow is a valid api_config_source/GRPC/V3/envoy_grpc ConfigSource
// (flow-style JSON-in-YAML) pointed at the "sds_cluster" static cluster.
const grpcSdsConfigFlow = `{resource_api_version: V3, api_config_source: {api_type: GRPC, transport_api_version: V3, grpc_services: [{envoy_grpc: {cluster_name: sds_cluster}}]}}`

// adsSdsConfigFlow is an ads-sourced ConfigSource — xds.ParseSDSConfig arm 1
// rejects this ("ads-sourced ConfigSource unsupported").
const adsSdsConfigFlow = `{resource_api_version: V3, ads: {}}`

// loadSDSBootstrapAndDialer parses yaml, builds the cluster.Manager over it
// (t.TempDir() baseDir), and returns (bs, dialer) — the pair NewSDSProvider
// needs. Fatalf's on a broken precondition (malformed YAML / cluster-manager
// build failure), mirroring mustConstruct's helper-failure discipline.
func loadSDSBootstrapAndDialer(t *testing.T, yaml string) (*bootstrap.Bootstrap, *grpcclient.Dialer) {
	t.Helper()
	bs, err := bootstrap.Load(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("bootstrap.Load: %v", err)
	}
	cm, err := cluster.NewManagerWithBaseDir(bs.Proto, t.TempDir(), bs.Stats)
	if err != nil {
		t.Fatalf("cluster.NewManagerWithBaseDir: %v", err)
	}
	return bs, grpcclient.New(cm)
}

func TestNewSDSProvider_NoSDS_ReturnsNilNil(t *testing.T) {
	bs, dialer := loadSDSBootstrapAndDialer(t, validYAML)
	provider, err := NewSDSProvider(dialer, bs, "", bs.Stats)
	if err != nil {
		t.Errorf("NewSDSProvider: got error %v, want nil", err)
	}
	if provider != nil {
		t.Errorf("NewSDSProvider: got non-nil provider, want nil")
	}
}

func TestNewSDSProvider_ArmSevenEmptyNode_Rejects(t *testing.T) {
	yaml := fmt.Sprintf(sdsListenerYAMLTemplate, "", "", grpcSdsConfigFlow, 1)
	bs, dialer := loadSDSBootstrapAndDialer(t, yaml)
	_, err := NewSDSProvider(dialer, bs, t.TempDir(), bs.Stats)
	if err == nil {
		t.Fatal("NewSDSProvider: want error for empty node.id/node.cluster, got nil")
	}
	const want = "node.id and node.cluster are required for SDS"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("NewSDSProvider error = %q; want substring %q", err.Error(), want)
	}
}

func TestNewSDSProvider_AdsConfigSource_Rejects(t *testing.T) {
	yaml := fmt.Sprintf(sdsListenerYAMLTemplate, "test-node", "test-cluster", adsSdsConfigFlow, 1)
	bs, dialer := loadSDSBootstrapAndDialer(t, yaml)
	_, err := NewSDSProvider(dialer, bs, t.TempDir(), bs.Stats)
	if err == nil {
		t.Fatal("NewSDSProvider: want error for ads-sourced ConfigSource, got nil")
	}
	const want = "ads-sourced"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("NewSDSProvider error = %q; want substring %q", err.Error(), want)
	}
}

// genLeafSelfSignedCert builds a self-signed leaf cert/key PEM pair.
// internal/xds's parseSecret only runs crypto/tls.X509KeyPair over the
// delivered bytes (no chain/handshake validation), so a bare self-signed
// leaf is sufficient — mirrors internal/tls/config_test.go's `pki` helper
// (minus the CA, which is not needed here).
func genLeafSelfSignedCert(t *testing.T) (certPEM, keyPEM []byte, serial *big.Int) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	serial = big.NewInt(4242)
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "sds.envoy-go.test"},
		DNSNames:     []string{"sds.envoy-go.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("x509.MarshalPKCS8PrivateKey: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, serial
}

func TestNewSDSProvider_Success_FetchesDeliveredCertificate(t *testing.T) {
	certPEM, keyPEM, wantSerial := genLeafSelfSignedCert(t)
	srv := sdsserver.New(t, sdsserver.WithSecret("server_cert", certPEM, keyPEM))

	_, portStr, err := net.SplitHostPort(srv.Addr())
	if err != nil {
		t.Fatalf("net.SplitHostPort(%q): %v", srv.Addr(), err)
	}
	port, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		t.Fatalf("strconv.ParseUint(%q): %v", portStr, err)
	}

	yaml := fmt.Sprintf(sdsListenerYAMLTemplate, "test-node", "test-cluster", grpcSdsConfigFlow, uint32(port))
	bs, dialer := loadSDSBootstrapAndDialer(t, yaml)

	provider, err := NewSDSProvider(dialer, bs, t.TempDir(), bs.Stats)
	if err != nil {
		t.Fatalf("NewSDSProvider: %v", err)
	}
	if provider == nil {
		t.Fatal("NewSDSProvider: got nil provider, want non-nil")
	}

	cert, err := provider.FetchInitialCertificate(context.Background(), "server_cert")
	if err != nil {
		t.Fatalf("FetchInitialCertificate: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("FetchInitialCertificate: cert.Certificate is empty")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("x509.ParseCertificate: %v", err)
	}
	if leaf.SerialNumber.Cmp(wantSerial) != 0 {
		t.Errorf("delivered leaf serial = %v, want %v", leaf.SerialNumber, wantSerial)
	}
}
