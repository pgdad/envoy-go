package boot

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	stdtls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
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

// This file drives the SDS subsystem END-TO-END through the SAME wiring
// cmd/envoy-go/main.go uses: bootstrap.Load → cluster manager → grpcclient
// dialer → NewSDSProvider → Construct → lm.Start → a live TLS client
// handshake. Before it, every joint was tested in isolation (NewSDSProvider
// pre-scan here, Provider fetch classification in internal/xds, the tls
// apply-point with a fakeProvider in internal/tls, and the Docker-based
// differential fixtures 0103/0108/0109) — but no in-process test proved the
// joints compose: that a bootstrap carrying an SDS-bound TLS context actually
// boots, serves the SDS-delivered material on a real handshake, and — the
// security-relevant half — that a fetch failure FAILS THE BOOT (the ADR-0280
// fail-closed departure) rather than leaving a listener serving with an
// unpopulated trust store, and that the phase-66 CVC "pool substitution"
// (Design A) really REPLACES the inline default CA rather than merging it.
//
// Determinism: ephemeral ports everywhere; the only timeout-driven test uses
// the config's own initial_fetch_timeout (200ms) against a Silent() server —
// no sleeps.

// --- test PKI -------------------------------------------------------------

type testCA struct {
	certPEM []byte
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
}

// newTestCA builds a self-signed CA (IsCA, CertSign) usable as a trust anchor.
func newTestCA(t *testing.T, cn string) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate (CA %s): %v", cn, err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate: %v", err)
	}
	return &testCA{
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		cert:    cert,
		key:     key,
	}
}

// issueClientCert issues a ClientAuth leaf signed by ca and returns it as a
// ready-to-send stdtls.Certificate.
func issueClientCert(t *testing.T, ca *testCA, cn string) stdtls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate (leaf %s): %v", cn, err)
	}
	return stdtls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}
}

// genServerCertPEM builds a self-signed ServerAuth leaf for dnsName.
func genServerCertPEM(t *testing.T, dnsName string) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("x509.MarshalPKCS8PrivateKey: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}

// --- harness --------------------------------------------------------------

// startEchoBackend binds a TCP echo server so accepted (post-handshake)
// connections can prove end-to-end data flow through tcp_proxy: a client that
// reads back its own bytes was REALLY accepted, not just left half-open.
func startEchoBackend(t *testing.T) (port uint32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				// Explicit read/write loop, NOT io.Copy(c, c): with src == dst
				// the same-socket splice(2) fast path can stall (see the netConn
				// wrapper note in internal/filter/tcpproxy/filter.go).
				defer func() { _ = c.Close() }()
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						if _, werr := c.Write(buf[:n]); werr != nil {
							return
						}
					}
					if err != nil {
						return
					}
				}
			}(c)
		}
	}()
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("net.SplitHostPort: %v", err)
	}
	p, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		t.Fatalf("strconv.ParseUint: %v", err)
	}
	return uint32(p)
}

// sdsE2EYAMLTemplate is the end-to-end bootstrap: one TLS listener whose
// tcp_proxy targets a LIVE echo cluster, plus the sds_cluster. Verbs:
// %t = require_client_certificate; %s = flow-style common_tls_context body;
// %d = echo backend port; %d = sds server port.
const sdsE2EYAMLTemplate = `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_tls_e2e
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
              require_client_certificate: %t
              common_tls_context: %s
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
                    socket_address: { address: 127.0.0.1, port_value: %d }
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

// constructSDS runs the REAL main.go SDS boot sequence over yaml with a single
// shared baseDir and returns (lm, err) — err is Construct's (or
// NewSDSProvider's) boot outcome.
func constructSDS(t *testing.T, yaml, baseDir string) (*listener.Manager, error) {
	t.Helper()
	bs, err := bootstrap.Load(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("bootstrap.Load: %v", err)
	}
	cm, err := cluster.NewManagerWithBaseDir(bs.Proto, baseDir, bs.Stats)
	if err != nil {
		t.Fatalf("cluster.NewManagerWithBaseDir: %v", err)
	}
	dm := drain.New(30 * time.Second)
	httpClient := httpclient.New(httpclient.Options{Timeout: 30 * time.Second})
	dialer := grpcclient.New(cm)
	tracingProvider := NewTracingProvider(dialer, httpClient, cm, bs.Stats)
	provider, err := NewSDSProvider(dialer, bs, baseDir, bs.Stats)
	if err != nil {
		return nil, err
	}
	return Construct(bs, cm, baseDir, false, nil, dm, httpClient, tracingProvider, provider)
}

// startAndAddr Starts lm, registers Stop, and returns the single listener's
// resolved address.
func startAndAddr(t *testing.T, lm *listener.Manager) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := lm.Start(ctx); err != nil {
		t.Fatalf("lm.Start: %v", err)
	}
	t.Cleanup(lm.Stop)
	ls := lm.Listeners()
	if len(ls) != 1 {
		t.Fatalf("Listeners() = %d entries, want 1", len(ls))
	}
	return ls[0].Addr
}

// sdsPort extracts the numeric port of an sdsserver.
func sdsPort(t *testing.T, addr string) uint32 {
	t.Helper()
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("net.SplitHostPort(%q): %v", addr, err)
	}
	p, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		t.Fatalf("strconv.ParseUint(%q): %v", portStr, err)
	}
	return uint32(p)
}

// echoRoundTrip proves end-to-end acceptance: writes a probe through the TLS
// conn and expects tcp_proxy + the echo backend to return it.
func echoRoundTrip(t *testing.T, conn *stdtls.Conn) {
	t.Helper()
	const probe = "sds-e2e-ping"
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(probe)); err != nil {
		t.Fatalf("conn.Write: %v", err)
	}
	buf := make([]byte, len(probe))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("echo read: %v", err)
	}
	if string(buf) != probe {
		t.Fatalf("echo = %q, want %q", string(buf), probe)
	}
}

// writeServerCertFiles writes the static server cert pair into baseDir under
// the fixed names the YAML bodies below reference.
func writeServerCertFiles(t *testing.T, baseDir string) (rootPool *x509.CertPool) {
	t.Helper()
	certPEM, keyPEM := genServerCertPEM(t, "static.envoy-go.test")
	if err := os.WriteFile(filepath.Join(baseDir, "server_cert.pem"), certPEM, 0o600); err != nil {
		t.Fatalf("write server_cert.pem: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "server_key.pem"), keyPEM, 0o600); err != nil {
		t.Fatalf("write server_key.pem: %v", err)
	}
	rootPool = x509.NewCertPool()
	if !rootPool.AppendCertsFromPEM(certPEM) {
		t.Fatal("AppendCertsFromPEM(server cert)")
	}
	return rootPool
}

// --- tests ----------------------------------------------------------------

// TestSDSEndToEnd_ServerCertViaSDS_HandshakeServesDeliveredLeaf: the phase-60.2
// shape, end-to-end in-process. The SDS management server delivers the server
// leaf; the booted listener must present EXACTLY that leaf on a live handshake,
// and the connection must proxy bytes (echo round-trip).
func TestSDSEndToEnd_ServerCertViaSDS_HandshakeServesDeliveredLeaf(t *testing.T) {
	certPEM, keyPEM, wantSerial := genLeafSelfSignedCert(t)
	srv := sdsserver.New(t, sdsserver.WithSecret("server_cert", certPEM, keyPEM))
	echoPort := startEchoBackend(t)

	ctc := fmt.Sprintf(`{tls_certificate_sds_secret_configs: [{name: server_cert, sds_config: %s}]}`, grpcSdsConfigFlow)
	yaml := fmt.Sprintf(sdsE2EYAMLTemplate, false, ctc, echoPort, sdsPort(t, srv.Addr()))

	lm, err := constructSDS(t, yaml, t.TempDir())
	if err != nil {
		t.Fatalf("SDS boot: %v", err)
	}
	addr := startAndAddr(t, lm)

	rootPool := x509.NewCertPool()
	if !rootPool.AppendCertsFromPEM(certPEM) {
		t.Fatal("AppendCertsFromPEM(SDS leaf)")
	}
	conn, err := stdtls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, &stdtls.Config{
		ServerName: "sds.envoy-go.test",
		RootCAs:    rootPool,
		MinVersion: stdtls.VersionTLS12,
	})
	if err != nil {
		t.Fatalf("TLS dial against the SDS-boot listener: %v", err)
	}
	defer func() { _ = conn.Close() }()

	peers := conn.ConnectionState().PeerCertificates
	if len(peers) == 0 {
		t.Fatal("no peer certificates presented")
	}
	if peers[0].SerialNumber.Cmp(wantSerial) != 0 {
		t.Errorf("served leaf serial = %v, want the SDS-delivered %v", peers[0].SerialNumber, wantSerial)
	}
	echoRoundTrip(t, conn)
}

// TestSDSEndToEnd_ValidationContextViaSDS_mTLS: the phase-65 shape, end-to-end.
// The SDS server delivers the trusted CA; require_client_certificate: true. A
// client presenting a leaf signed by the DELIVERED CA is accepted (echo
// round-trip); a client presenting a leaf signed by a DIFFERENT CA — force-sent
// via GetClientCertificate so the test cannot go vacuously green by silently
// withholding the cert — is refused; a certless client is refused.
func TestSDSEndToEnd_ValidationContextViaSDS_mTLS(t *testing.T) {
	caServed := newTestCA(t, "sds-served-ca")
	caOther := newTestCA(t, "never-trusted-ca")
	srv := sdsserver.New(t, sdsserver.WithValidationContext("validation_ca", caServed.certPEM))
	echoPort := startEchoBackend(t)
	baseDir := t.TempDir()
	rootPool := writeServerCertFiles(t, baseDir)

	ctc := fmt.Sprintf(`{tls_certificates: [{certificate_chain: {filename: server_cert.pem}, private_key: {filename: server_key.pem}}], validation_context_sds_secret_config: {name: validation_ca, sds_config: %s}}`, grpcSdsConfigFlow)
	yaml := fmt.Sprintf(sdsE2EYAMLTemplate, true, ctc, echoPort, sdsPort(t, srv.Addr()))

	lm, err := constructSDS(t, yaml, baseDir)
	if err != nil {
		t.Fatalf("SDS boot: %v", err)
	}
	addr := startAndAddr(t, lm)

	baseCfg := func() *stdtls.Config {
		return &stdtls.Config{
			ServerName: "static.envoy-go.test",
			RootCAs:    rootPool,
			// TLS 1.2 makes the server's client-cert verdict part of the
			// handshake itself, so accept/reject is observable at Dial time.
			MinVersion: stdtls.VersionTLS12,
			MaxVersion: stdtls.VersionTLS12,
		}
	}

	t.Run("leaf signed by the SDS-delivered CA is accepted", func(t *testing.T) {
		goodCert := issueClientCert(t, caServed, "good-client")
		cfg := baseCfg()
		cfg.GetClientCertificate = func(*stdtls.CertificateRequestInfo) (*stdtls.Certificate, error) {
			return &goodCert, nil
		}
		conn, err := stdtls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, cfg)
		if err != nil {
			t.Fatalf("mTLS dial with a leaf signed by the SDS-delivered CA failed: %v", err)
		}
		defer func() { _ = conn.Close() }()
		echoRoundTrip(t, conn)
	})

	t.Run("leaf signed by an untrusted CA is refused", func(t *testing.T) {
		badCert := issueClientCert(t, caOther, "bad-client")
		cfg := baseCfg()
		cfg.GetClientCertificate = func(*stdtls.CertificateRequestInfo) (*stdtls.Certificate, error) {
			return &badCert, nil // FORCE-send (the vacuous-green trap)
		}
		conn, err := stdtls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, cfg)
		if err == nil {
			_ = conn.Close()
			t.Fatal("mTLS dial with a leaf signed by an UNTRUSTED CA succeeded; the SDS-delivered CA must be the only trust anchor")
		}
	})

	t.Run("certless client is refused", func(t *testing.T) {
		conn, err := stdtls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, baseCfg())
		if err == nil {
			_ = conn.Close()
			t.Fatal("mTLS dial with NO client certificate succeeded; require_client_certificate: true must refuse it")
		}
	})
}

// TestSDSEndToEnd_CVC_PoolSubstitution: the phase-66 Design A security
// property, end-to-end. Under a combined_validation_context the SDS-delivered
// CA pool REPLACES the inline default_validation_context.trusted_ca — it must
// NOT be merged. A client leaf signed by the SDS-delivered CA is accepted; a
// client leaf signed by the INLINE DEFAULT CA must be REFUSED. If this test
// ever accepts the default-CA leaf, the pool substitution silently became a
// merge (falsifying ADR-0287's equivalence theorem premise P3/P5).
func TestSDSEndToEnd_CVC_PoolSubstitution(t *testing.T) {
	caServed := newTestCA(t, "sds-served-ca")
	caDefault := newTestCA(t, "inline-default-ca")
	srv := sdsserver.New(t, sdsserver.WithValidationContext("validation_ca", caServed.certPEM))
	echoPort := startEchoBackend(t)
	baseDir := t.TempDir()
	rootPool := writeServerCertFiles(t, baseDir)
	if err := os.WriteFile(filepath.Join(baseDir, "ca_default.pem"), caDefault.certPEM, 0o600); err != nil {
		t.Fatalf("write ca_default.pem: %v", err)
	}

	ctc := fmt.Sprintf(`{tls_certificates: [{certificate_chain: {filename: server_cert.pem}, private_key: {filename: server_key.pem}}], combined_validation_context: {default_validation_context: {trusted_ca: {filename: ca_default.pem}}, validation_context_sds_secret_config: {name: validation_ca, sds_config: %s}}}`, grpcSdsConfigFlow)
	yaml := fmt.Sprintf(sdsE2EYAMLTemplate, true, ctc, echoPort, sdsPort(t, srv.Addr()))

	lm, err := constructSDS(t, yaml, baseDir)
	if err != nil {
		t.Fatalf("SDS boot: %v", err)
	}
	addr := startAndAddr(t, lm)

	baseCfg := func() *stdtls.Config {
		return &stdtls.Config{
			ServerName: "static.envoy-go.test",
			RootCAs:    rootPool,
			MinVersion: stdtls.VersionTLS12,
			MaxVersion: stdtls.VersionTLS12,
		}
	}

	t.Run("leaf signed by the SDS-delivered CA is accepted", func(t *testing.T) {
		goodCert := issueClientCert(t, caServed, "good-client")
		cfg := baseCfg()
		cfg.GetClientCertificate = func(*stdtls.CertificateRequestInfo) (*stdtls.Certificate, error) {
			return &goodCert, nil
		}
		conn, err := stdtls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, cfg)
		if err != nil {
			t.Fatalf("mTLS dial with a leaf signed by the SDS-delivered CA failed: %v", err)
		}
		defer func() { _ = conn.Close() }()
		echoRoundTrip(t, conn)
	})

	t.Run("leaf signed by the INLINE DEFAULT CA is refused (pool substituted, not merged)", func(t *testing.T) {
		defaultCert := issueClientCert(t, caDefault, "default-ca-client")
		cfg := baseCfg()
		cfg.GetClientCertificate = func(*stdtls.CertificateRequestInfo) (*stdtls.Certificate, error) {
			return &defaultCert, nil // FORCE-send
		}
		conn, err := stdtls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, cfg)
		if err == nil {
			_ = conn.Close()
			t.Fatal("mTLS dial with a leaf signed by the INLINE DEFAULT CA succeeded — the SDS pool was MERGED with (not substituted for) the default, falsifying phase 66 Design A")
		}
	})
}

// TestSDSEndToEnd_FetchFailure_BootFailsClosed pins the ADR-0280-family
// fail-closed posture at the REAL apply-point: when the initial SDS fetch
// cannot complete, the boot itself must FAIL — no listener may come up
// serving with an unpopulated trust store
// (D-RCCF-FETCHFAIL-POSTURE was RESOLVED at the phase-67 SPEC — probe P1,
// {server-cert, validation-context} × {silent, unreachable}, all four cells
// identical: the reference init-holds (port unbound), then at
// initial_fetch_timeout starts workers and binds, then fails closed
// per-connection (downstream_context_secrets_not_ready); envoy-go's own
// posture is boot-FAIL — ADR-0280 family, characterization corrected at
// ADR-0289 — and this test is its integration-level pin).
func TestSDSEndToEnd_FetchFailure_BootFailsClosed(t *testing.T) {
	echoPort := startEchoBackend(t)

	// A 200ms initial_fetch_timeout keeps the timeout arm fast + deterministic
	// (config-driven, not sleep-driven).
	const shortTimeoutSdsFlow = `{resource_api_version: V3, initial_fetch_timeout: 0.2s, api_config_source: {api_type: GRPC, transport_api_version: V3, grpc_services: [{envoy_grpc: {cluster_name: sds_cluster}}]}}`

	t.Run("silent SDS server: validation_context fetch times out, boot fails", func(t *testing.T) {
		caPEM := newTestCA(t, "unused-ca").certPEM
		srv := sdsserver.New(t, sdsserver.WithValidationContext("validation_ca", caPEM), sdsserver.Silent())
		baseDir := t.TempDir()
		writeServerCertFiles(t, baseDir)

		ctc := fmt.Sprintf(`{tls_certificates: [{certificate_chain: {filename: server_cert.pem}, private_key: {filename: server_key.pem}}], validation_context_sds_secret_config: {name: validation_ca, sds_config: %s}}`, shortTimeoutSdsFlow)
		yaml := fmt.Sprintf(sdsE2EYAMLTemplate, true, ctc, echoPort, sdsPort(t, srv.Addr()))

		lm, err := constructSDS(t, yaml, baseDir)
		if err == nil {
			t.Fatal("boot with a SILENT SDS server succeeded — a fetch timeout must FAIL the boot (fail-closed), not serve with an unpopulated trust store")
		}
		if lm != nil {
			t.Errorf("Construct returned a non-nil manager alongside the error")
		}
		if !strings.Contains(err.Error(), "initial fetch timed out") {
			t.Errorf("boot error = %q, want it to mention the initial-fetch timeout", err.Error())
		}
	})

	t.Run("unreachable SDS server: server-cert fetch fails, boot fails", func(t *testing.T) {
		// Bind + close a port so the dial refuses fast.
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("net.Listen: %v", err)
		}
		deadPort := sdsPort(t, lis.Addr().String())
		_ = lis.Close()

		ctc := fmt.Sprintf(`{tls_certificate_sds_secret_configs: [{name: server_cert, sds_config: %s}]}`, shortTimeoutSdsFlow)
		yaml := fmt.Sprintf(sdsE2EYAMLTemplate, false, ctc, echoPort, deadPort)

		lm, err := constructSDS(t, yaml, t.TempDir())
		if err == nil {
			t.Fatal("boot with an UNREACHABLE SDS server succeeded — the fetch failure must FAIL the boot (fail-closed)")
		}
		if lm != nil {
			t.Errorf("Construct returned a non-nil manager alongside the error")
		}
		if !strings.Contains(err.Error(), "SDS") {
			t.Errorf("boot error = %q, want it to identify the SDS fetch", err.Error())
		}
	})
}
